package api

// APIResponse represents a generic API response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// StatusResponse represents the response for the status endpoint
type StatusResponse struct {
	Status           string `json:"status"`
	ManticoreHealthy bool   `json:"manticore_healthy"`
	DocumentsLoaded  int    `json:"documents_loaded"`
	VectorizerReady  bool   `json:"vectorizer_ready"`
}

// ReindexResponse represents the response for the reindex endpoint
type ReindexResponse struct {
	Message        string `json:"message"`
	DocumentsCount int    `json:"documents_count"`
	IndexingTime   string `json:"indexing_time"`
}
