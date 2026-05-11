// Package config предоставляет конфигурацию для приложения.
package config

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config содержит все настройки приложения.
type Config struct {
	RunAddr              string        `env:"RUN_ADDRESS" env-default:":8080" flag:"a" flag-desc:"address and port to run server"`
	BaseURL              string        `env:"BASE_URL" flag:"b" flag-desc:"base URL for shortened URLs"`
	LogLevel             string        `env:"LOG_LEVEL" env-default:"info" flag:"l" flag-desc:"log level"`
	FileStoragePath      string        `env:"FILE_STORAGE_PATH" env-default:"storage.json" flag:"f" flag-desc:"file storage path"`
	DatabaseDSN          string        `env:"DATABASE_URI" flag:"d" flag-desc:"database connection DSN"`
	SecretKey            string        `env:"SECRET_KEY" flag:"k" flag-desc:"secret key for JWT"`
	WorkerQueueSize      int           `env:"WORKER_QUEUE_SIZE" env-default:"100" flag:"q" flag-desc:"worker queue size"`
	WorkerCount          int           `env:"WORKER_COUNT" env-default:"5" flag:"w" flag-desc:"number of workers"`
	WorkerTimeout        time.Duration `env:"WORKER_TIMEOUT" env-default:"30s" flag:"t" flag-desc:"worker operation timeout"`
	JWTExpiry            time.Duration `env:"JWT_EXPIRY" env-default:"24h" flag:"jwt-expiry" flag-desc:"JWT token expiration time"`
	AccrualSystemAddress string        `env:"ACCRUAL_SYSTEM_ADDRESS" flag:"r" flag-desc:"address of the accrual calculation system"`
	MLServiceURL         string        `env:"ML_SERVICE_URL" env-default:"http://localhost:8000" flag:"m" flag-desc:"ML service URL"`
}

// ParseFlags парсит флаги командной строки и переменные окружения.
func ParseFlags() Config {
	var cfg Config

	cfg.RunAddr = ":8080"
	cfg.LogLevel = "info"
	cfg.FileStoragePath = "storage.json"
	cfg.WorkerQueueSize = 100
	cfg.WorkerCount = 5
	cfg.WorkerTimeout = 30 * time.Second
	cfg.JWTExpiry = 24 * time.Hour
	cfg.MLServiceURL = "http://localhost:8000"

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		log.Printf("Warning: error reading environment variables: %v", err)
	}

	// Backward compatibility: часть компонентов использует DATABASE_URL.
	if cfg.DatabaseDSN == "" {
		cfg.DatabaseDSN = os.Getenv("DATABASE_URL")
	}

	flag.StringVar(&cfg.RunAddr, "a", cfg.RunAddr, "address and port to run server")
	flag.StringVar(&cfg.BaseURL, "b", cfg.BaseURL, "base URL for shortened URLs")
	flag.StringVar(&cfg.LogLevel, "l", cfg.LogLevel, "log level")
	flag.StringVar(&cfg.FileStoragePath, "f", cfg.FileStoragePath, "file storage path")
	flag.StringVar(&cfg.DatabaseDSN, "d", cfg.DatabaseDSN, "database connection DSN")
	flag.StringVar(&cfg.SecretKey, "k", cfg.SecretKey, "secret key for JWT")
	flag.IntVar(&cfg.WorkerQueueSize, "q", cfg.WorkerQueueSize, "worker queue size")
	flag.IntVar(&cfg.WorkerCount, "w", cfg.WorkerCount, "number of workers")
	flag.DurationVar(&cfg.WorkerTimeout, "t", cfg.WorkerTimeout, "worker operation timeout")
	flag.DurationVar(&cfg.JWTExpiry, "jwt-expiry", cfg.JWTExpiry, "JWT token expiration time")
	flag.StringVar(&cfg.AccrualSystemAddress, "r", cfg.AccrualSystemAddress, "address of the accrual calculation system")
	flag.StringVar(&cfg.MLServiceURL, "m", cfg.MLServiceURL, "ML service URL (e.g., http://localhost:8000)")

	flag.Parse()

	return cfg
}
