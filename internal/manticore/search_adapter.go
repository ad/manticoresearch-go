package manticore

import (
	"fmt"
	"log"

	"github.com/ad/manticoresearch-go/internal/models"
)

// SearchAdapter provides a unified search interface for both client types
type SearchAdapter struct {
	client ClientInterface
}

// NewSearchAdapter creates a new search adapter
func NewSearchAdapter(client ClientInterface) *SearchAdapter {
	return &SearchAdapter{
		client: client,
	}
}

// BasicSearch performs basic text matching search
func (sa *SearchAdapter) BasicSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	switch client := sa.client.(type) {
	case *manticoreHTTPClient:
		return sa.basicSearchHTTP(client, query, page, pageSize)
	default:
		return nil, fmt.Errorf("unsupported client type")
	}
}

// FullTextSearch performs full-text search
func (sa *SearchAdapter) FullTextSearch(query string, page, pageSize int) (*models.SearchResponse, error) {
	switch client := sa.client.(type) {
	case *manticoreHTTPClient:
		return sa.fullTextSearchHTTP(client, query, page, pageSize)
	default:
		return nil, fmt.Errorf("unsupported client type")
	}
}

// GetAllDocuments retrieves all documents
func (sa *SearchAdapter) GetAllDocuments() ([]*models.Document, error) {
	return sa.client.GetAllDocuments()
}

// basicSearchHTTP performs basic search using the HTTP client
func (sa *SearchAdapter) basicSearchHTTP(client *manticoreHTTPClient, query string, page, pageSize int) (*models.SearchResponse, error) {
	log.Printf("BasicSearch (HTTP): query='%s', page=%d, pageSize=%d", query, page, pageSize)

	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)

	// Create basic search request
	searchReq := client.CreateBasicSearchRequest("documents", query, limit, offset)

	// Execute search
	resp, err := client.SearchWithRequest(searchReq)
	if err != nil {
		log.Printf("BasicSearch (HTTP): search failed: %v", err)
		return nil, fmt.Errorf("basic search failed: %v", err)
	}

	log.Printf("BasicSearch (HTTP): got response with %d hits", resp.Hits.Total)

	// Convert to internal format
	results, err := client.convertSearchResponseWithScores(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search response: %v", err)
	}

	log.Printf("BasicSearch (HTTP): returning %d results", len(results))

	return &models.SearchResponse{
		Documents: results,
		Total:     int(resp.Hits.Total),
		Page:      page,
		Mode:      string(models.SearchModeBasic),
	}, nil
}

// fullTextSearchHTTP performs full-text search using the HTTP client
func (sa *SearchAdapter) fullTextSearchHTTP(client *manticoreHTTPClient, query string, page, pageSize int) (*models.SearchResponse, error) {
	log.Printf("FullTextSearch (HTTP): query='%s', page=%d, pageSize=%d", query, page, pageSize)

	offset := int32((page - 1) * pageSize)
	limit := int32(pageSize)

	// Create full-text search request
	searchReq := client.CreateFullTextSearchRequest("documents", query, limit, offset)

	// Execute search
	resp, err := client.SearchWithRequest(searchReq)
	if err != nil {
		log.Printf("FullTextSearch (HTTP): search failed: %v", err)
		return nil, fmt.Errorf("full-text search failed: %v", err)
	}

	log.Printf("FullTextSearch (HTTP): got response with %d hits", resp.Hits.Total)

	// Convert to internal format
	results, err := client.convertSearchResponseWithScores(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to convert search response: %v", err)
	}

	log.Printf("FullTextSearch (HTTP): returning %d results", len(results))

	return &models.SearchResponse{
		Documents: results,
		Total:     int(resp.Hits.Total),
		Page:      page,
		Mode:      string(models.SearchModeFullText),
	}, nil
}
