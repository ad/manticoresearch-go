package manticore

import (
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// ClientInterface defines the contract for Manticore client implementations
type ClientInterface interface {
	// Connection management
	WaitForReady(timeout time.Duration) error
	HealthCheck() error
	Close() error
	IsConnected() bool

	// Schema operations
	CreateSchema(aiConfig *models.AISearchConfig) error
	ResetDatabase() error
	TruncateTables() error

	// Document operations
	IndexDocument(doc *models.Document, vector []float64) error
	IndexDocuments(documents []*models.Document, vectors [][]float64) error

	// Search operations (for ClientInterface compatibility)
	Search(query string, mode models.SearchMode, page, pageSize int) (*models.SearchResponse, error)
	GetAllDocuments() ([]*models.Document, error)

	// HTTP-specific search operations
	SearchWithRequest(request SearchRequest) (*SearchResponse, error)

	// AI search operations
	AISearch(query string, model string, limit, offset int) (*SearchResponse, error)
	GenerateEmbedding(text string, model string) ([]float64, error)
}

// HTTPClientConfig holds configuration for the HTTP client
type HTTPClientConfig struct {
	BaseURL              string
	Timeout              time.Duration
	MaxIdleConns         int
	MaxIdleConnsPerHost  int
	IdleConnTimeout      time.Duration
	RetryConfig          RetryConfig
	CircuitBreakerConfig CircuitBreakerConfig
	BulkConfig           BulkConfig
}

// BulkConfig holds configuration for bulk operations
type BulkConfig struct {
	BatchSize           int           // Number of documents per batch
	MaxConcurrentBatch  int           // Maximum concurrent batch operations
	StreamingThreshold  int           // Threshold for using streaming operations
	ProgressLogInterval int           // Log progress every N documents
	BatchTimeout        time.Duration // Timeout for individual batch operations
}

// DefaultBulkConfig returns default bulk operation configuration
func DefaultBulkConfig() BulkConfig {
	return BulkConfig{
		BatchSize:           100,
		MaxConcurrentBatch:  3,
		StreamingThreshold:  1000,
		ProgressLogInterval: 500,
		BatchTimeout:        60 * time.Second,
	}
}

// DefaultHTTPClientConfig returns a default configuration
func DefaultHTTPClientConfig(baseURL string) HTTPClientConfig {
	return HTTPClientConfig{
		BaseURL:              baseURL,
		Timeout:              60 * time.Second,
		MaxIdleConns:         20,
		MaxIdleConnsPerHost:  10,
		IdleConnTimeout:      90 * time.Second,
		RetryConfig:          DefaultRetryConfig(),
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		BulkConfig:           DefaultBulkConfig(),
	}
}

// JSON API request/response types
type SearchRequest struct {
	Index  string                 `json:"index"`
	Query  map[string]interface{} `json:"query"`
	Limit  int32                  `json:"limit,omitempty"`
	Offset int32                  `json:"offset,omitempty"`
}

type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Hits     struct {
		Total         int32  `json:"total"`
		TotalRelation string `json:"total_relation"`
		Hits          []struct {
			Index  string                 `json:"_index"`
			ID     int64                  `json:"_id"`
			Score  float32                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type SQLRequest struct {
	Query string `json:"query"`
}

type SQLResponse struct {
	Data  []map[string]interface{} `json:"data,omitempty"`
	Total int                      `json:"total,omitempty"`
	Error string                   `json:"error,omitempty"`
}

type ReplaceRequest struct {
	Index string                 `json:"index"`
	ID    int64                  `json:"id"`
	Doc   map[string]interface{} `json:"doc"`
}

type ReplaceResponse struct {
	Index   string `json:"_index"`
	ID      int64  `json:"_id"`
	Created bool   `json:"created"`
	Result  string `json:"result"`
	Status  int    `json:"status"`
}

type BulkRequest struct {
	Replace *ReplaceRequest `json:"replace,omitempty"`
}

type BulkResponse struct {
	Items []struct {
		Replace *struct {
			Index   string `json:"_index"`
			ID      int64  `json:"_id"`
			Created bool   `json:"created"`
			Result  string `json:"result"`
			Status  int    `json:"status"`
			Error   string `json:"error,omitempty"`
		} `json:"replace,omitempty"`
	} `json:"items"`
	Errors bool `json:"errors"`
}

// AI search request/response types
type AISearchRequest struct {
	Index  string                 `json:"index"`
	Query  map[string]interface{} `json:"query"`
	Limit  int                    `json:"limit,omitempty"`
	Offset int                    `json:"offset,omitempty"`
}

type EmbeddingRequest struct {
	Text  string `json:"text"`
	Model string `json:"model"`
}

type EmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
	Model     string    `json:"model"`
	Tokens    int       `json:"tokens,omitempty"`
}

// SearchResultProcessor handles search result processing and ranking
type SearchResultProcessor struct {
	client ClientInterface
}
