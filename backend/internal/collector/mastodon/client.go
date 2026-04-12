// Package mastodon предоставляет клиент для работы с API Mastodon.
package mastodon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client представляет HTTP клиент для взаимодействия с Mastodon API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// NewClient создаёт новый экземпляр клиента Mastodon.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		token: token,
	}
}

// doRequest выполняет HTTP запрос к Mastodon API.
func (c *Client) doRequest(endpoint string, result interface{}) error {
	url := c.baseURL + endpoint

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("создание запроса: %w", err)
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("выполнение запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("неожиданный статус: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("декодирование ответа: %w", err)
	}

	return nil
}

// GetTrendingTags возвращает трендовые хэштеги.
func (c *Client) GetTrendingTags(limit int) ([]TrendingTag, error) {
	var tags []TrendingTag
	endpoint := fmt.Sprintf("/api/v1/trends/tags?limit=%d", limit)

	if err := c.doRequest(endpoint, &tags); err != nil {
		return nil, fmt.Errorf("получение трендовых хэштегов: %w", err)
	}

	return tags, nil
}

// GetTrendingStatuses возвращает трендовые посты.
func (c *Client) GetTrendingStatuses(limit int) ([]Status, error) {
	var statuses []Status
	endpoint := fmt.Sprintf("/api/v1/trends/statuses?limit=%d", limit)

	if err := c.doRequest(endpoint, &statuses); err != nil {
		return nil, fmt.Errorf("получение трендовых постов: %w", err)
	}

	return statuses, nil
}

// GetTrendingLinks возвращает трендовые ссылки.
func (c *Client) GetTrendingLinks(limit int) ([]TrendingLink, error) {
	var links []TrendingLink
	endpoint := fmt.Sprintf("/api/v1/trends/links?limit=%d", limit)

	if err := c.doRequest(endpoint, &links); err != nil {
		return nil, fmt.Errorf("получение трендовых ссылок: %w", err)
	}

	return links, nil
}

// GetSuggestions возвращает рекомендуемые аккаунты.
func (c *Client) GetSuggestions(limit int) ([]Suggestion, error) {
	var suggestions []Suggestion
	endpoint := fmt.Sprintf("/api/v2/suggestions?limit=%d", limit)

	if err := c.doRequest(endpoint, &suggestions); err != nil {
		return nil, fmt.Errorf("получение рекомендаций: %w", err)
	}

	return suggestions, nil
}

// GetStatus возвращает один пост по ID.
func (c *Client) GetStatus(statusID string) (*Status, error) {
	var status Status
	endpoint := fmt.Sprintf("/api/v1/statuses/%s", statusID)

	if err := c.doRequest(endpoint, &status); err != nil {
		return nil, fmt.Errorf("получение поста: %w", err)
	}

	return &status, nil
}

// GetStatusContext возвращает контекст поста (предки и потомки).
func (c *Client) GetStatusContext(statusID string) (map[string]interface{}, error) {
	var context map[string]interface{}
	endpoint := fmt.Sprintf("/api/v1/statuses/%s/context", statusID)

	if err := c.doRequest(endpoint, &context); err != nil {
		return nil, fmt.Errorf("получение контекста: %w", err)
	}

	return context, nil
}
