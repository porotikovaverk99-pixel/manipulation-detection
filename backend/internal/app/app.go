// Package app инициализирует и запускает приложение.
package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/config"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/handler"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/server"
	"go.uber.org/zap"
)

// App представляет основное приложение.
type App struct {
	config           *config.Config
	logger           *zap.Logger
	server           *server.Server
	db               *repository.PostgresDB
	pingHandler      *handler.PingHandler
	analyzeHandler   *handler.AnalyzeHandler
	analysisHandler  *handler.AnalysisHandler
	ingestHandler    *handler.IngestionHandler
	casesHandler     *handler.CasesHandler
	dashboardHandler *handler.DashboardHandler
}

// NewApp создаёт новое приложение.
func NewApp() (*App, error) {

	cfg := config.ParseFlags()

	if err := logger.Initialize(cfg.LogLevel); err != nil {
		return nil, fmt.Errorf("initialize logger: %w", err)
	}
	zapLogger := logger.Log

	// URL ML сервиса из конфига или переменной окружения
	mlURL := cfg.MLServiceURL
	if mlURL == "" {
		mlURL = "http://localhost:8000"
	}

	// Инициализируем хендлеры
	db, err := repository.NewPostgresDB(cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}

	pingHandler := handler.NewPingHandler()
	analyzeHandler := handler.NewAnalyzeHandler(mlURL)
	analysisHandler := handler.NewAnalysisHandler(db)
	ingestHandler := handler.NewIngestionHandler(db)
	casesHandler := handler.NewCasesHandler(db)
	dashboardHandler := handler.NewDashboardHandler(db)

	// Создаем сервер
	srv := server.New(cfg.RunAddr)

	return &App{
		config:           &cfg,
		logger:           zapLogger,
		server:           srv,
		db:               db,
		pingHandler:      pingHandler,
		analyzeHandler:   analyzeHandler,
		analysisHandler:  analysisHandler,
		ingestHandler:    ingestHandler,
		casesHandler:     casesHandler,
		dashboardHandler: dashboardHandler,
	}, nil
}

// setupRoutes настраивает маршруты.
func (a *App) setupRoutes() {
	// Регистрируем маршруты
	a.server.Handle("/ping", a.pingHandler.Ping())
	a.server.Handle("/api/analyze", a.analyzeHandler.Analyze())
	a.server.Get("/api/analysis/summary", a.corsMiddleware(a.analysisHandler.Summary()))
	a.server.Get("/api/ingestion/runs", a.corsMiddleware(a.ingestHandler.ListRuns()))
	a.server.Get("/api/cases", a.corsMiddleware(a.casesHandler.List()))
	a.server.Get("/api/cases/summary", a.corsMiddleware(a.casesHandler.Summary()))
	a.server.Get("/api/dashboard/metrics", a.corsMiddleware(a.dashboardHandler.Metrics()))
	a.server.Get("/api/cases/{id}", a.corsMiddleware(a.casesHandler.Detail()))
	a.server.Get("/api/cases/{id}/scores", a.corsMiddleware(a.casesHandler.Scores()))
	a.server.Get("/api/model-comparison", a.corsMiddleware(a.casesHandler.ModelComparison()))

	// Добавляем CORS для фронтенда
	a.server.Handle("/api/", a.corsMiddleware(a.analyzeHandler.Analyze()))
}

// corsMiddleware добавляет CORS заголовки.
func (a *App) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// Run запускает приложение.
func (a *App) Run() error {
	a.setupRoutes()

	// Канал для ошибок сервера
	serverErr := make(chan error, 1)

	// Запускаем сервер в горутине
	go func() {
		log.Printf("Сервер запущен на http://localhost:8080")
		log.Printf("Доступные эндпоинты:")
		log.Printf("  GET  /ping")
		log.Printf("  POST /api/analyze")
		log.Printf("  GET  /api/analysis/summary")
		log.Printf("  GET  /api/ingestion/runs")
		log.Printf("  GET  /api/cases")
		log.Printf("  GET  /api/cases/summary")
		log.Printf("  GET  /api/dashboard/metrics")
		log.Printf("  GET  /api/cases/{id}")
		log.Printf("  GET  /api/cases/{id}/scores")
		log.Printf("  GET  /api/model-comparison")

		if err := a.server.Run(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Ожидаем сигналы завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		log.Printf("Ошибка сервера: %v", err)
		return err
	case sig := <-sigChan:
		log.Printf("Получен сигнал: %v", sig)
		return a.shutdown()
	}
}

// shutdown gracefully завершает работу.
func (a *App) shutdown() error {
	log.Println("Завершение работы сервера...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.server.Shutdown(ctx); err != nil {
		log.Printf("Ошибка при завершении: %v", err)
		return err
	}

	log.Println("Сервер успешно остановлен")
	return nil
}

// Close освобождает ресурсы приложения (логгер).
func (a *App) Close() {
	if a.db != nil {
		_ = a.db.Close()
	}
	_ = a.logger.Sync()
}
