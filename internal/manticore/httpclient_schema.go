package manticore

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Schema operations

// executeSQL executes a SQL command using the /cli endpoint with comprehensive logging
func (mc *manticoreHTTPClient) executeSQL(query string) error {
	startTime := time.Now()
	log.Printf("[SQL] Starting execution: %s", query)

	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		// Use /cli endpoint with form data instead of /sql with JSON
		log.Printf("[SQL] [REQUEST] POST %s/cli - Query: %s", mc.baseURL, query)

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/cli", strings.NewReader(query))
		if err != nil {
			log.Printf("[SQL] [ERROR] Failed to create HTTP request for query '%s': %v", query, err)
			return fmt.Errorf("failed to create SQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := mc.httpClient.Do(req)
		requestDuration := time.Since(requestStartTime)

		if err != nil {
			log.Printf("[SQL] [ERROR] HTTP request failed for query '%s' after %v: %v", query, requestDuration, err)
			return fmt.Errorf("SQL request failed: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[SQL] [ERROR] Failed to read response body for query '%s' after %v: %v", query, requestDuration, err)
			return fmt.Errorf("failed to read SQL response: %v", err)
		}

		log.Printf("[SQL] [RESPONSE] HTTP %d - Response size: %d bytes - Duration: %v", resp.StatusCode, len(body), requestDuration)
		if len(body) > 0 {
			log.Printf("[SQL] [RESPONSE] Body: %s", string(body))
		}

		if resp.StatusCode >= 400 {
			log.Printf("[SQL] [ERROR] SQL execution failed for query '%s': HTTP %d, %s", query, resp.StatusCode, string(body))
			return fmt.Errorf("SQL execution failed: HTTP %d, %s", resp.StatusCode, string(body))
		}

		// /cli endpoint returns plain text response, not JSON
		bodyStr := string(body)

		// Check for errors in the text response
		if strings.Contains(bodyStr, "ERROR") || strings.Contains(bodyStr, "error") {
			log.Printf("[SQL] [ERROR] SQL error in response for query '%s': %s", query, bodyStr)
			return fmt.Errorf("SQL error: %s", bodyStr)
		}

		// Log successful execution
		log.Printf("[SQL] [SUCCESS] Query executed successfully: %s - Duration: %v", query, requestDuration)
		log.Printf("[SQL] [SUCCESS] Response: %s", bodyStr)

		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := mc.circuitBreakerWithRetry.Execute(ctx, mc.baseURL+"/sql", "POST", operation)

	totalDuration := time.Since(startTime)

	// Record metrics
	if mc.metricsCollector != nil {
		mc.metricsCollector.RecordRequest("SQL", totalDuration, err == nil, "")
		mc.metricsCollector.RecordSchemaOperation()
	}

	if err != nil {
		log.Printf("[SQL] [FINAL] Query failed after %v: %s - Error: %v", totalDuration, query, err)
		if mc.logger != nil {
			mc.logger.LogOperation("SQL", totalDuration, false, fmt.Sprintf("Query: %s, Error: %v", query, err))
		}
	} else {
		log.Printf("[SQL] [FINAL] Query completed successfully after %v: %s", totalDuration, query)
		if mc.logger != nil {
			mc.logger.LogOperation("SQL", totalDuration, true, fmt.Sprintf("Query: %s", query))
		}
	}

	return err
}

// CreateSchema creates the database schema for Manticore Search
func (c *manticoreHTTPClient) CreateSchema(aiConfig *models.AISearchConfig) error {
	log.Println("Creating Manticore Search schema...")

	// Drop existing tables first
	tables := []string{"documents", "documents_basic", "documents_fulltext", "documents_vector", "documents_hybrid"}
	for _, table := range tables {
		dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s", table)
		if err := c.executeSQL(dropQuery); err != nil {
			log.Printf("Warning: Failed to drop table %s: %v", table, err)
		}
	}

	// Determine AI model to use
	aiModel := "sentence-transformers/all-MiniLM-L6-v2" // Default fallback
	if aiConfig != nil && aiConfig.Model != "" {
		aiModel = aiConfig.Model
		log.Printf("Using configured AI model: %s", aiModel)
	} else {
		log.Printf("Using default AI model: %s", aiModel)
	}

	// Create unified documents table with Auto Embeddings using configurable model
	// Correct syntax for Auto Embeddings in Manticore Search 13.11+ (all in CREATE TABLE)
	createTableQuery := fmt.Sprintf(`
		CREATE TABLE documents (
			id BIGINT,
			title TEXT,
			content TEXT,
			url TEXT,
			content_vector FLOAT_VECTOR KNN_TYPE='hnsw' HNSW_SIMILARITY='cosine' MODEL_NAME='%s' FROM='content'
		) ENGINE='columnar'`, aiModel)

	log.Printf("Executing schema creation query with Auto Embeddings: %s", createTableQuery)

	if err := c.executeSQL(createTableQuery); err != nil {
		log.Printf("Schema creation failed: %v", err)
		return fmt.Errorf("failed to create documents table: %v", err)
	}

	log.Printf("Successfully created documents table with Auto Embeddings model: %s", aiModel)

	// Create documents_vector table for traditional vector search (fallback)
	vectorTableQuery := `
		CREATE TABLE documents_vector (
			id BIGINT,
			title TEXT,
			url TEXT,
			vector_data TEXT
		) ENGINE='columnar'`

	log.Printf("Creating documents_vector table: %s", vectorTableQuery)

	if err := c.executeSQL(vectorTableQuery); err != nil {
		log.Printf("Vector table creation failed: %v", err)
		return fmt.Errorf("failed to create documents_vector table: %v", err)
	}

	log.Println("Schema creation completed successfully with AI model:", aiModel)
	return nil
}

// ResetDatabase drops existing tables to start fresh
func (mc *manticoreHTTPClient) ResetDatabase() error {
	log.Printf("[SCHEMA] [RESET] Starting database reset...")

	// Drop existing tables using SQL API (ignore errors if tables don't exist)
	dropDocuments := "DROP TABLE IF EXISTS documents"
	if err := mc.executeSQL(dropDocuments); err != nil {
		log.Printf("[SCHEMA] [RESET] [WARNING] Failed to drop documents table: %v", err)
	}

	// Also drop old documents_vector table if it exists (from previous schema)
	dropVectors := "DROP TABLE IF EXISTS documents_vector"
	if err := mc.executeSQL(dropVectors); err != nil {
		log.Printf("[SCHEMA] [RESET] [WARNING] Failed to drop documents_vector table: %v", err)
	}

	log.Printf("[SCHEMA] [RESET] [SUCCESS] Database reset completed")
	return nil
}

// TruncateTables clears all data from existing tables
func (mc *manticoreHTTPClient) TruncateTables() error {
	log.Printf("[SCHEMA] [TRUNCATE] Starting table truncation...")

	// Truncate documents table (now includes auto-generated vectors)
	truncateDocuments := "TRUNCATE TABLE documents"
	if err := mc.executeSQL(truncateDocuments); err != nil {
		log.Printf("[SCHEMA] [TRUNCATE] [WARNING] Failed to truncate documents table: %v", err)
	}

	log.Printf("[SCHEMA] [TRUNCATE] [SUCCESS] Table truncation completed")
	return nil
}
