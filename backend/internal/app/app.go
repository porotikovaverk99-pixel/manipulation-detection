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
	config          *config.Config
	logger          *zap.Logger
	server          *server.Server
	db              *repository.PostgresDB
	pingHandler     *handler.PingHandler
	healthHandler   *handler.HealthHandler
	analyzeHandler  *handler.AnalyzeHandler
	analysisHandler *handler.AnalysisHandler
	ingestHandler   *handler.IngestionHandler
	casesHandler    *handler.CasesHandler
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
	healthHandler := handler.NewHealthHandler(db, mlURL)
	analyzeHandler := handler.NewAnalyzeHandler(mlURL)
	analysisHandler := handler.NewAnalysisHandler(db)
	ingestHandler := handler.NewIngestionHandler(db)
	casesHandler := handler.NewCasesHandler(db)

	// Создаем сервер
	srv := server.New(cfg.RunAddr)

	return &App{
		config:          &cfg,
		logger:          zapLogger,
		server:          srv,
		db:              db,
		pingHandler:     pingHandler,
		healthHandler:   healthHandler,
		analyzeHandler:  analyzeHandler,
		analysisHandler: analysisHandler,
		ingestHandler:   ingestHandler,
		casesHandler:    casesHandler,
	}, nil
}

// setupRoutes настраивает маршруты.
func (a *App) setupRoutes() {
	a.server.Use(handler.NewAuditMiddleware(a.db))
	a.server.Use(logger.HTTPLogger)

	// Регистрируем маршруты
	a.server.Handle("/ping", a.pingHandler.Ping())
	a.server.Get("/health", a.corsMiddleware(a.healthHandler.Health()))
	a.server.Handle("/api/analyze", a.analyzeHandler.Analyze())
	a.server.Get("/api/analysis/summary", a.corsMiddleware(a.analysisHandler.Summary()))
	a.server.Get("/api/ingestion/runs", a.corsMiddleware(a.ingestHandler.ListRuns()))
	a.server.Get("/api/cases", a.corsMiddleware(a.casesHandler.List()))
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

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
	runRequestID := fmt.Sprintf("cli-api-%d", time.Now().UTC().UnixNano())
	a.saveLifecycleAudit(runRequestID, "started", nil, map[string]interface{}{
		"addr": a.config.RunAddr,
	})

	// Канал для ошибок сервера
	serverErr := make(chan error, 1)

	// Запускаем сервер в горутине
	go func() {
		log.Printf("Сервер запущен на http://localhost:8080")
		log.Printf("Доступные эндпоинты:")
		log.Printf("  GET  /ping")
		log.Printf("  GET  /health")
		log.Printf("  POST /api/analyze")
		log.Printf("  GET  /api/analysis/summary")
		log.Printf("  GET  /api/ingestion/runs")
		log.Printf("  GET  /api/cases")
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
		a.saveLifecycleAudit(runRequestID, "failed", err, map[string]interface{}{
			"addr": a.config.RunAddr,
		})
		return err
	case sig := <-sigChan:
		log.Printf("Получен сигнал: %v", sig)
		err := a.shutdown()
		status := "succeeded"
		if err != nil {
			status = "failed"
		}
		a.saveLifecycleAudit(runRequestID, status, err, map[string]interface{}{
			"addr":   a.config.RunAddr,
			"signal": sig.String(),
		})
		return err
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

func (a *App) saveLifecycleAudit(requestID, status string, err error, payload map[string]interface{}) {
	if a.db == nil {
		return
	}
	hostname, hostErr := os.Hostname()
	if hostErr != nil || hostname == "" {
		hostname = "local"
	}
	event := repository.AuditEventRecord{
		ActorType:  "cli",
		ActorID:    hostname,
		Action:     "cli.api",
		EntityType: "cli_command",
		EntityID:   "api",
		Status:     status,
		RequestID:  requestID,
		Payload:    payload,
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	if _, saveErr := a.db.SaveAuditEvent(event); saveErr != nil {
		log.Printf("save API lifecycle audit event failed: %v", saveErr)
	}
}
