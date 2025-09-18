package manticore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Schema operations

// executeSQL executes a SQL command using the /sql endpoint with comprehensive logging
func (mc *manticoreHTTPClient) executeSQL(query string) error {
	startTime := time.Now()
	log.Printf("[SQL] Starting execution: %s", query)

	operation := func(ctx context.Context) error {
		requestStartTime := time.Now()

		sqlRequest := SQLRequest{Query: query}

		reqBody, err := json.Marshal(sqlRequest)
		if err != nil {
			log.Printf("[SQL] [ERROR] Failed to marshal request for query '%s': %v", query, err)
			return fmt.Errorf("failed to marshal SQL request: %v", err)
		}

		log.Printf("[SQL] [REQUEST] POST %s/sql - Body size: %d bytes", mc.baseURL, len(reqBody))
		log.Printf("[SQL] [REQUEST] Payload: %s", string(reqBody))

		req, err := http.NewRequestWithContext(ctx, "POST", mc.baseURL+"/sql", bytes.NewReader(reqBody))
		if err != nil {
			log.Printf("[SQL] [ERROR] Failed to create HTTP request for query '%s': %v", query, err)
			return fmt.Errorf("failed to create SQL request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

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

		// Parse response to check for SQL errors
		var sqlResponse SQLResponse
		if err := json.Unmarshal(body, &sqlResponse); err != nil {
			// If we can't parse as JSON, check if it's a plain text response
			if strings.Contains(string(body), "error") || strings.Contains(string(body), "ERROR") {
				log.Printf("[SQL] [ERROR] SQL error in response for query '%s': %s", query, string(body))
				return fmt.Errorf("SQL error: %s", string(body))
			}
			// Otherwise assume success for non-JSON responses
			log.Printf("[SQL] [SUCCESS] Query executed successfully (non-JSON response): %s - Duration: %v", query, requestDuration)
			return nil
		}

		if sqlResponse.Error != "" {
			log.Printf("[SQL] [ERROR] SQL error in parsed response for query '%s': %s", query, sqlResponse.Error)
			return fmt.Errorf("SQL error: %s", sqlResponse.Error)
		}

		// Log successful execution with performance metrics
		rowCount := len(sqlResponse.Data)
		log.Printf("[SQL] [SUCCESS] Query executed successfully: %s - Duration: %v - Rows affected/returned: %d", query, requestDuration, rowCount)

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

// CreateSchema creates the necessary tables for documents and vectors
func (mc *manticoreHTTPClient) CreateSchema() error {
	schemaStartTime := time.Now()
	log.Printf("[SCHEMA] [CREATE] Starting schema creation using Manticore JSON API...")

	// Drop existing tables if they exist (ignore errors for first run)
	log.Printf("[SCHEMA] [CREATE] Performing cleanup of existing tables...")
	if err := mc.ResetDatabase(); err != nil {
		log.Printf("[SCHEMA] [CREATE] [WARNING] Cleanup failed (this is normal for first run): %v", err)
	}

	// Create full-text search table
	log.Printf("[SCHEMA] [CREATE] Creating full-text search table 'documents'...")
	fullTextQuery := "CREATE TABLE documents (id bigint, title text, content text, url string)"
	if err := mc.executeSQL(fullTextQuery); err != nil {
		log.Printf("[SCHEMA] [CREATE] [ERROR] Failed to create documents table: %v", err)
		return fmt.Errorf("failed to create documents table: %v", err)
	}
	log.Printf("[SCHEMA] [CREATE] [SUCCESS] Created full-text search table 'documents'")

	// Create vector search table
	log.Printf("[SCHEMA] [CREATE] Creating vector search table 'documents_vector'...")
	vectorQuery := "CREATE TABLE documents_vector (id bigint, title string, url string, vector_data text)"
	if err := mc.executeSQL(vectorQuery); err != nil {
		log.Printf("[SCHEMA] [CREATE] [ERROR] Failed to create documents_vector table: %v", err)
		return fmt.Errorf("failed to create documents_vector table: %v", err)
	}
	log.Printf("[SCHEMA] [CREATE] [SUCCESS] Created vector search table 'documents_vector'")

	totalDuration := time.Since(schemaStartTime)
	log.Printf("[SCHEMA] [CREATE] [FINAL] Schema creation completed successfully in %v", totalDuration)
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

	// Truncate documents table
	truncateDocuments := "TRUNCATE TABLE documents"
	if err := mc.executeSQL(truncateDocuments); err != nil {
		log.Printf("[SCHEMA] [TRUNCATE] [WARNING] Failed to truncate documents table: %v", err)
	}

	// Truncate vectors table
	truncateVectors := "TRUNCATE TABLE documents_vector"
	if err := mc.executeSQL(truncateVectors); err != nil {
		log.Printf("[SCHEMA] [TRUNCATE] [WARNING] Failed to truncate documents_vector table: %v", err)
	}

	log.Printf("[SCHEMA] [TRUNCATE] [SUCCESS] Table truncation completed")
	return nil
}
