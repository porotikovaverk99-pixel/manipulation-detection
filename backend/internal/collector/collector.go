// Package collector предоставляет функциональность для сбора данных из социальных сетей.
package collector

import (
	"fmt"
	"log"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/collector/mastodon"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// Collector собирает данные из социальных сетей и сохраняет в БД.
type Collector struct {
	db       *repository.PostgresDB
	config   *Config
	mastodon *mastodon.Client
}

// NewCollector создаёт новый экземпляр сборщика данных.
func NewCollector(db *repository.PostgresDB, cfg *Config) *Collector {
	return &Collector{
		db:       db,
		config:   cfg,
		mastodon: mastodon.NewClient(cfg.MastodonBaseURL, cfg.MastodonToken),
	}
}

// CollectTrendingTags собирает трендовые хэштеги.
func (c *Collector) CollectTrendingTags() error {
	log.Println("  📈 Получение трендовых хэштегов...")

	tags, err := c.mastodon.GetTrendingTags(20)
	if err != nil {
		return err
	}

	log.Printf("  📝 Получено %d трендовых хэштегов", len(tags))

	for _, tag := range tags {
		if err := c.db.SaveTrendingTag(1, &tag); err != nil {
			log.Printf("  ⚠️ Ошибка сохранения хэштега #%s: %v", tag.Name, err)
		}
	}

	return nil
}

// CollectTrendingStatuses собирает трендовые посты.
func (c *Collector) CollectTrendingStatuses() error {
	log.Println("  📰 Получение трендовых постов...")

	statuses, err := c.mastodon.GetTrendingStatuses(1000)
	if err != nil {
		return err
	}

	log.Printf("  📝 Получено %d трендовых постов", len(statuses))

	for _, status := range statuses {
		// Сохраняем аккаунт
		accountID, err := c.db.SaveAccount(1, &status.Account)
		if err != nil {
			log.Printf("  ⚠️ Ошибка сохранения аккаунта @%s: %v", status.Account.Username, err)
			continue
		}

		// Сохраняем пост
		if err := c.db.SavePost(1, &status, accountID); err != nil {
			log.Printf("  ⚠️ Ошибка сохранения поста %s: %v", status.ID, err)
		}
	}

	return nil
}

// CollectTrendingLinks собирает трендовые ссылки.
func (c *Collector) CollectTrendingLinks() error {
	log.Println("  🔗 Получение трендовых ссылок...")

	links, err := c.mastodon.GetTrendingLinks(20)
	if err != nil {
		return err
	}

	log.Printf("  📝 Получено %d трендовых ссылок", len(links))

	for _, link := range links {
		if err := c.db.SaveTrendingLink(1, &link); err != nil {
			log.Printf("  ⚠️ Ошибка сохранения ссылки %s: %v", link.URL, err)
		}
	}

	return nil
}

// CollectSuggestions собирает рекомендуемые аккаунты.
func (c *Collector) CollectSuggestions() error {
	log.Println("  👤 Получение рекомендуемых аккаунтов...")

	suggestions, err := c.mastodon.GetSuggestions(20)
	if err != nil {
		return err
	}

	log.Printf("  📝 Получено %d рекомендаций", len(suggestions))

	for _, sug := range suggestions {
		if _, err := c.db.SaveAccount(1, &sug.Account); err != nil {
			log.Printf("  ⚠️ Ошибка сохранения рекомендованного аккаунта @%s: %v", sug.Account.Username, err)
		}
	}

	return nil
}

// CollectAll выполняет сбор всех типов данных и анализ.
func (c *Collector) CollectAll() error {
	log.Println("🔄 Начинаем полный сбор данных...")

	// Сбор данных из Mastodon
	if err := c.CollectTrendingTags(); err != nil {
		log.Printf("❌ Ошибка сбора хэштегов: %v", err)
	}

	if err := c.CollectTrendingStatuses(); err != nil {
		log.Printf("❌ Ошибка сбора постов: %v", err)
	}

	if err := c.CollectTrendingLinks(); err != nil {
		log.Printf("❌ Ошибка сбора ссылок: %v", err)
	}

	// Анализ собранных постов
	if err := c.AnalyzeUnanalyzedPosts(); err != nil {
		log.Printf("❌ Ошибка анализа постов: %v", err)
	}

	log.Println("✅ Полный сбор и анализ данных завершён")
	return nil
}

// AnalyzeUnanalyzedPosts анализирует посты, которые ещё не были проанализированы.
func (c *Collector) AnalyzeUnanalyzedPosts() error {
	log.Println("  🔍 Поиск неанализированных постов...")

	posts, err := c.db.GetUnanalyzedPosts(1000)
	if err != nil {
		return fmt.Errorf("get unanalyzed posts: %w", err)
	}

	if len(posts) == 0 {
		log.Println("  ✅ Нет новых постов для анализа")
		return nil
	}

	log.Printf("  📝 Найдено %d постов для анализа", len(posts))

	mlClient := NewMLClient(c.config.MLURL)

	for _, post := range posts {
		log.Printf("  🔬 Анализ поста #%d", post.ID)

		// Очищаем HTML теги из контента
		cleanText := stripHTMLTags(post.Content)
		if cleanText == "" {
			log.Printf("  ⚠️ Пустой текст в посте #%d, пропускаем", post.ID)
			continue
		}

		// Получаем пост + аккаунт ОДНИМ запросом
		fullData, err := c.db.GetPostWithAccount(post.ID)
		if err != nil {
			log.Printf("  ⚠️ Ошибка получения данных поста #%d: %v", post.ID, err)
			continue
		}

		mlResp, err := mlClient.AnalyzeText(
			int(fullData.ID),
			cleanText,
			int(fullData.AccountID),
			fullData.Username,
			fullData.PublishedAt,
			fullData.FollowersCount,
			fullData.AccountCreatedAt,
		)

		if err != nil {
			log.Printf("  ❌ Ошибка анализа поста #%d: %v", post.ID, err)
			continue
		}

		// Определяем приоритет эскалации
		escalationPriority := GetEscalationPriority(
			mlResp.ManipulationScore,
			mlResp.ConfidenceScore,
		)

		// КОНВЕРТИРУЕМ MLResponse В ФОРМАТ ДЛЯ БД
		repoMLResp := &repository.MLResponse{
			ManipulationScore:        mlResp.ManipulationScore,
			ConfidenceScore:          mlResp.ConfidenceScore,
			CoordinationContribution: mlResp.CoordinationContribution,
			TemporalContribution:     mlResp.TemporalContribution,
			NarrativeContribution:    mlResp.NarrativeContribution,
			ConfidenceNote:           mlResp.ConfidenceNote,
			KeyEvidence:              mlResp.KeyEvidence,
			Tactics:                  mlResp.Tactics,
		}

		// Сохраняем результат в БД
		if err := c.db.SaveAnalysisResult(post.ID, repoMLResp, escalationPriority); err != nil {
			log.Printf("  ❌ Ошибка сохранения результата: %v", err)
			continue
		}

		log.Printf("  ✅ Пост #%d: манипуляция %.1f%%, приоритет %d",
			post.ID, mlResp.ManipulationScore*100, escalationPriority)
	}

	return nil
}

// stripHTMLTags удаляет HTML теги из текста.
func stripHTMLTags(text string) string {
	// Простая очистка от HTML тегов
	result := ""
	inTag := false
	for _, ch := range text {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result += string(ch)
		}
	}
	return result
}
