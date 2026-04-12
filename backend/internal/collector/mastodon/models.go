// Package mastodon предоставляет модели данных для API Mastodon.
package mastodon

import "time"

// Account представляет аккаунт пользователя в Mastodon.
type Account struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Acct           string    `json:"acct"`
	DisplayName    string    `json:"display_name"`
	Bot            bool      `json:"bot"`
	CreatedAt      time.Time `json:"created_at"`
	FollowersCount int       `json:"followers_count"`
	FollowingCount int       `json:"following_count"`
	StatusesCount  int       `json:"statuses_count"`
	URL            string    `json:"url"`
	Avatar         string    `json:"avatar"`
	Note           string    `json:"note"`
	Fields         []Field   `json:"fields"`
}

// Field представляет поле профиля аккаунта.
type Field struct {
	Name       string `json:"name"`
	Value      string `json:"value"`
	VerifiedAt string `json:"verified_at"`
}

// Status представляет пост в Mastodon.
type Status struct {
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	Content          string    `json:"content"`
	Language         string    `json:"language"`
	URL              string    `json:"url"`
	Account          Account   `json:"account"`
	FavouritesCount  int       `json:"favourites_count"`
	ReblogsCount     int       `json:"reblogs_count"`
	RepliesCount     int       `json:"replies_count"`
	Mentions         []Mention `json:"mentions"`
	Tags             []Tag     `json:"tags"`
	MediaAttachments []Media   `json:"media_attachments"`
}

// Mention представляет упоминание другого аккаунта.
type Mention struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Acct     string `json:"acct"`
	URL      string `json:"url"`
}

// Tag представляет хэштег.
type Tag struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Media представляет медиа-вложение.
type Media struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	PreviewURL  string `json:"preview_url"`
	Description string `json:"description"`
}

// TrendingTag представляет трендовый хэштег.
type TrendingTag struct {
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	History []History `json:"history"`
}

// TrendingLink представляет трендовую ссылку.
type TrendingLink struct {
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	ProviderName string    `json:"provider_name"`
	Image        string    `json:"image"`
	History      []History `json:"history"`
}

// History представляет историю популярности по дням.
type History struct {
	Day      string `json:"day"`
	Accounts string `json:"accounts"`
	Uses     string `json:"uses"`
}

// Suggestion представляет рекомендуемый аккаунт.
type Suggestion struct {
	Source  string   `json:"source"`
	Sources []string `json:"sources"`
	Account Account  `json:"account"`
}
