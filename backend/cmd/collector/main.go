// Пакет main предоставляет точку входа для сбора данных из социальных сетей.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/audittrail"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}
	logFile, err := applog.ConfigureStandardLog("collector")
	if err != nil {
		log.Printf("configure file logging failed: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = os.Getenv("DATABASE_URI")
	}
	if connStr == "" {
		connStr = "postgres://postgres:password@localhost:5432/manipulation_detection?sslmode=disable"
	}

	db, err := repository.NewPostgresDB(connStr)
	if err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	defer db.Close()

	mastodonToken := os.Getenv("MASTODON_TOKEN")
	mastodonBaseURL := os.Getenv("MASTODON_BASE_URL")
	if mastodonBaseURL == "" {
		mastodonBaseURL = "https://mastodon.social"
	}

	// В функции main, после создания коллектора, добавьте ML URL:
	mlURL := os.Getenv("ML_URL")
	if mlURL == "" {
		mlURL = "http://localhost:8000"
	}

	if mastodonToken == "" {
		log.Println("MASTODON_TOKEN не задан. Live-сбор может быть ограничен или недоступен.")
	}

	cfg := &collector.Config{
		MastodonBaseURL: mastodonBaseURL,
		MastodonToken:   mastodonToken,
		CollectInterval: 1 * time.Hour,
		LimitPerRequest: 50,
		MLURL:           mlURL, // Добавьте это поле в Config
	}

	c := collector.NewCollector(db, cfg)

	job := audittrail.NewCLIJob(db, "collector").
		WithSource("live", "mastodon", "", "").
		WithPayload(map[string]interface{}{
			"mastodon_base_url":        mastodonBaseURL,
			"collect_interval_seconds": int(cfg.CollectInterval.Seconds()),
			"limit_per_request":        cfg.LimitPerRequest,
			"ml_url":                   mlURL,
			"token_configured":         mastodonToken != "",
		})
	job.Start()
	defer job.FinishAndExit()

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("🚀 Запуск сборщика данных из Mastodon...")

	go func() {
		for {
			cycle := audittrail.NewCLIJob(db, "collector_collect_all").
				WithSource("live", "mastodon", "", "").
				WithPayload(map[string]interface{}{
					"mastodon_base_url": mastodonBaseURL,
					"limit_per_request": cfg.LimitPerRequest,
				})
			cycle.Start()
			if err := c.CollectAll(); err != nil {
				cycle.Failf("collector collect all failed: %v", err)
			}
			cycle.Finish()
			log.Printf("⏳ Следующий запуск через %v", cfg.CollectInterval)
			time.Sleep(cfg.CollectInterval)
		}
	}()

	<-stopChan
	job.Set("stop_signal", "received")
	log.Println("👋 Остановка сборщика данных")
}
