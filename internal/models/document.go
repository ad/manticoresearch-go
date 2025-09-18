package models

import "time"

// AISearchConfig holds configuration for AI search functionality
type AISearchConfig struct {
	Model   string        `json:"model"`
	Enabled bool          `json:"enabled"`
	Timeout time.Duration `json:"timeout"`
}

// Document represents a parsed markdown document
type Document struct {
	ID      int    `json:"id"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// SearchResult represents a search result with document and score
type SearchResult struct {
	Document *Document `json:"document"`
	Score    float64   `json:"score"`
}

// SearchResponse represents the response structure for search API
type SearchResponse struct {
	Documents []SearchResult `json:"documents"`
	Total     int            `json:"total"`
	Page      int            `json:"page"`
	Mode      string         `json:"mode"`
}

// AISearchResponse extends SearchResponse with AI-specific metadata
type AISearchResponse struct {
	SearchResponse
	AIModel        string `json:"ai_model,omitempty"`
	AIEnabled      bool   `json:"ai_enabled,omitempty"`
	FallbackUsed   bool   `json:"fallback_used,omitempty"`
	FallbackReason string `json:"fallback_reason,omitempty"`
}

// SearchMode represents the different search modes available
type SearchMode string

const (
	SearchModeBasic    SearchMode = "basic"
	SearchModeFullText SearchMode = "fulltext"
	SearchModeVector   SearchMode = "vector"
	SearchModeHybrid   SearchMode = "hybrid"
	SearchModeAI       SearchMode = "ai"
)
