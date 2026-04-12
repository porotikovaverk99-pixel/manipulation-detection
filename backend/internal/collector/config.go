// Пакет collector предоставляет функциональность для сбора данных из социальных сетей.
package collector

import "time"

type Config struct {
	MastodonBaseURL string
	MastodonToken   string
	CollectInterval time.Duration
	LimitPerRequest int
	MLURL           string // Добавьте эту строку
}
