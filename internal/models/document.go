package models

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

// SearchMode represents the different search modes available
type SearchMode string

const (
	SearchModeBasic    SearchMode = "basic"
	SearchModeFullText SearchMode = "fulltext"
	SearchModeVector   SearchMode = "vector"
	SearchModeHybrid   SearchMode = "hybrid"
)
