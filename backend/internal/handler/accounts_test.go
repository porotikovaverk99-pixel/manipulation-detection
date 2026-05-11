package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

// mockAccountsRepository implements accountsRepository interface for testing
type mockAccountsRepository struct {
	listAccountsFunc func(repository.AccountFilter) (repository.AccountsListResponse, error)
}

func (m *mockAccountsRepository) ListAccounts(filter repository.AccountFilter) (repository.AccountsListResponse, error) {
	if m.listAccountsFunc != nil {
		return m.listAccountsFunc(filter)
	}
	return repository.AccountsListResponse{
		Items: []repository.AccountListItem{},
		Total: 0,
		Page:  1,
		Limit: 50,
	}, nil
}

func TestAccountsHandler_List(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		mockResponse   repository.AccountsListResponse
		mockError      error
		expectedStatus int
		expectedTotal  int
	}{
		{
			name:        "successful list accounts",
			queryParams: "?page=1&limit=10",
			mockResponse: repository.AccountsListResponse{
				Items: []repository.AccountListItem{
					{
						ID:         1,
						Username:   "test_user",
						IsVerified: true,
					},
					{
						ID:         2,
						Username:   "test_user2",
						IsVerified: false,
					},
				},
				Total: 2,
				Page:  1,
				Limit: 10,
			},
			expectedStatus: http.StatusOK,
			expectedTotal:  2,
		},
		{
			name:           "empty list",
			queryParams:    "?page=1&limit=10",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 10},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with dataset filter",
			queryParams:    "?dataset_name=pheme&dataset_split=eventcv_large",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with search filter",
			queryParams:    "?search=test",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with verified filter true",
			queryParams:    "?verified=true",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with verified filter false",
			queryParams:    "?verified=false",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with bots filter",
			queryParams:    "?bots=true",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "with sort parameter",
			queryParams:    "?sort=followers_desc",
			mockResponse:   repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:        "invalid page - negative",
			queryParams: "?page=-1",
			mockResponse: repository.AccountsListResponse{
				Items: []repository.AccountListItem{},
				Total: 0,
				Page:  1,
				Limit: 50,
			},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:        "invalid limit - too high",
			queryParams: "?limit=500",
			mockResponse: repository.AccountsListResponse{
				Items: []repository.AccountListItem{},
				Total: 0,
				Page:  1,
				Limit: 50,
			},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:        "invalid limit - negative",
			queryParams: "?limit=-10",
			mockResponse: repository.AccountsListResponse{
				Items: []repository.AccountListItem{},
				Total: 0,
				Page:  1,
				Limit: 50,
			},
			expectedStatus: http.StatusOK,
			expectedTotal:  0,
		},
		{
			name:           "repository error",
			queryParams:    "?page=1&limit=10",
			mockError:      fmt.Errorf("database error"),
			expectedStatus: http.StatusInternalServerError,
			expectedTotal:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &mockAccountsRepository{
				listAccountsFunc: func(filter repository.AccountFilter) (repository.AccountsListResponse, error) {
					if tt.mockError != nil {
						return repository.AccountsListResponse{}, tt.mockError
					}
					return tt.mockResponse, nil
				},
			}

			handler := NewAccountsHandler(mockRepo)

			req := httptest.NewRequest(http.MethodGet, "/api/accounts"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handler.List()(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				var response repository.AccountsListResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if response.Total != tt.expectedTotal {
					t.Errorf("expected total %d, got %d", tt.expectedTotal, response.Total)
				}
			}
		})
	}
}

func TestParseIntQuery(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		queryVal string
		fallback int
		expected int
	}{
		{"valid integer", "page", "5", 1, 5},
		{"empty query", "page", "", 1, 1},
		{"invalid integer", "page", "abc", 1, 1},
		{"zero value", "page", "0", 1, 1},
		{"negative value", "page", "-10", 1, 1},
		{"large value", "limit", "1000", 50, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/accounts?"+tt.queryKey+"="+tt.queryVal, nil)
			result := parseIntQuery(req, tt.queryKey, tt.fallback)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestParseBoolQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"true string", "true", true},
		{"1 string", "1", true},
		{"yes string", "yes", true},
		{"on string", "on", true},
		{"True with capital", "True", true},
		{"TRUE uppercase", "TRUE", true},
		{"false string", "false", false},
		{"0 string", "0", false},
		{"no string", "no", false},
		{"off string", "off", false},
		{"empty string", "", false},
		{"random string", "something", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseBoolQuery(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAccountsHandler_ListWithFilters(t *testing.T) {
	mockRepo := &mockAccountsRepository{
		listAccountsFunc: func(filter repository.AccountFilter) (repository.AccountsListResponse, error) {
			// Verify filters are passed correctly
			if filter.DatasetName != "pheme" {
				t.Errorf("expected dataset_name='pheme', got '%s'", filter.DatasetName)
			}
			if filter.DatasetSplit != "eventcv_large" {
				t.Errorf("expected dataset_split='eventcv_large', got '%s'", filter.DatasetSplit)
			}
			if filter.Search != "test" {
				t.Errorf("expected search='test', got '%s'", filter.Search)
			}
			if !filter.OnlyVerified {
				t.Errorf("expected only_verified=true, got false")
			}
			if filter.Sort != "followers_desc" {
				t.Errorf("expected sort='followers_desc', got '%s'", filter.Sort)
			}
			return repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50}, nil
		},
	}

	handler := NewAccountsHandler(mockRepo)

	queryParams := "?dataset_name=pheme&dataset_split=eventcv_large&search=test&verified=true&sort=followers_desc&page=1&limit=10"
	req := httptest.NewRequest(http.MethodGet, "/api/accounts"+queryParams, nil)
	w := httptest.NewRecorder()

	handler.List()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestAccountsHandler_ListDefaultPagination(t *testing.T) {
	var capturedFilter repository.AccountFilter
	mockRepo := &mockAccountsRepository{
		listAccountsFunc: func(filter repository.AccountFilter) (repository.AccountsListResponse, error) {
			capturedFilter = filter
			return repository.AccountsListResponse{Items: []repository.AccountListItem{}, Total: 0, Page: 1, Limit: 50}, nil
		},
	}

	handler := NewAccountsHandler(mockRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/accounts", nil)
	w := httptest.NewRecorder()

	handler.List()(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify default values
	if capturedFilter.Limit != 50 {
		t.Errorf("expected default limit 50, got %d", capturedFilter.Limit)
	}
	if capturedFilter.Offset != 0 {
		t.Errorf("expected default offset 0, got %d", capturedFilter.Offset)
	}
}
