// Пакет main предоставляет точку входа для сбора данных из социальных сетей.
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Файл .env не найден, используем переменные окружения")
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = os.Getenv("DATABASE_URI")
	}
	if connStr == "" {
		connStr = "postgres://postgres:123@localhost:5432/manipulation_detection?sslmode=disable"
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

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("🚀 Запуск сборщика данных из Mastodon...")

	go func() {
		for {
			if err := c.CollectAll(); err != nil {
				log.Printf("❌ Ошибка при сборе: %v", err)
			}
			log.Printf("⏳ Следующий запуск через %v", cfg.CollectInterval)
			time.Sleep(cfg.CollectInterval)
		}
	}()

	<-stopChan
	log.Println("👋 Остановка сборщика данных")
}
