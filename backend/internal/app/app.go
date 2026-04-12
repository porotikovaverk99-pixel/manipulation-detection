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
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/server"
	"go.uber.org/zap"
)

// App представляет основное приложение.
type App struct {
	config         *config.Config
	logger         *zap.Logger
	server         *server.Server
	pingHandler    *handler.PingHandler
	analyzeHandler *handler.AnalyzeHandler
}

// NewApp создаёт новое приложение.
func NewApp() (*App, error) {

	cfg := config.ParseFlags()

	fmt.Fprintf(os.Stderr, "DEBUG: DSN from config = %q\n", cfg.DatabaseDSN)

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
	pingHandler := handler.NewPingHandler()
	analyzeHandler := handler.NewAnalyzeHandler(mlURL)

	// Создаем сервер
	srv := server.New(cfg.RunAddr)

	return &App{
		config:         &cfg,
		logger:         zapLogger,
		server:         srv,
		pingHandler:    pingHandler,
		analyzeHandler: analyzeHandler,
	}, nil
}

// setupRoutes настраивает маршруты.
func (a *App) setupRoutes() {
	// Регистрируем маршруты
	a.server.Handle("/ping", a.pingHandler.Ping())
	a.server.Handle("/api/analyze", a.analyzeHandler.Analyze())

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
	_ = a.logger.Sync()
}
