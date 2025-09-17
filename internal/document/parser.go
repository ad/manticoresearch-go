package document

import (
	"bufio"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ad/manticoresearch-go/internal/models"
)

// generateDocumentID generates a consistent unique ID based on file path
func generateDocumentID(filePath string) int {
	// Use MD5 hash of file path for consistent ID generation
	hash := md5.Sum([]byte(filePath))
	// Convert first 4 bytes of hash to int (positive number)
	id := binary.BigEndian.Uint32(hash[:4])
	// Ensure positive int by using absolute value
	return int(id & 0x7FFFFFFF)
}

// ParseMarkdownFile parses a single markdown file and extracts title, URL, and content
func ParseMarkdownFile(filePath string) (*models.Document, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	doc := &models.Document{}
	scanner := bufio.NewScanner(file)
	var contentLines []string
	titleFound := false
	urlFound := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Extract title from first # line
		if !titleFound && strings.HasPrefix(line, "#") {
			doc.Title = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			titleFound = true
			continue
		}

		// Extract URL from **URL:** line
		if !urlFound && strings.HasPrefix(line, "**URL:**") {
			urlPart := strings.TrimSpace(strings.TrimPrefix(line, "**URL:**"))
			doc.URL = urlPart
			urlFound = true
			continue
		}

		// Skip empty lines at the beginning
		if line == "" && len(contentLines) == 0 {
			continue
		}

		// Collect content lines (everything else)
		if titleFound && urlFound {
			contentLines = append(contentLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	// Join content lines
	doc.Content = strings.TrimSpace(strings.Join(contentLines, "\n"))

	// Basic validation (URL will be validated later after it's set)
	if doc.Title == "" {
		return nil, fmt.Errorf("validation failed for %s: title is required", filePath)
	}
	if doc.Content == "" {
		return nil, fmt.Errorf("validation failed for %s: content is required", filePath)
	}

	return doc, nil
}

// validateDocument checks if the document has required fields
func validateDocument(doc *models.Document) error {
	if doc.Title == "" {
		return fmt.Errorf("title is required")
	}
	if doc.URL == "" {
		return fmt.Errorf("URL is required")
	}
	if doc.Content == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

// ScanDataDirectory scans the ./data directory for markdown files and parses them
func ScanDataDirectory(dataDir string) ([]*models.Document, error) {
	var documents []*models.Document

	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		doc, parseErr := ParseMarkdownFile(path)
		if parseErr != nil {
			// Log error but continue processing other files
			fmt.Printf("Warning: Failed to parse %s: %v\n", path, parseErr)
			return nil
		}

		// Generate unique ID based on file path hash for consistency
		doc.ID = generateDocumentID(path)

		// Use file path as URL if not already set from document content
		if doc.URL == "" {
			doc.URL = path
		}

		// Final validation after URL is set
		if err := validateDocument(doc); err != nil {
			fmt.Printf("Warning: Document validation failed for %s: %v\n", path, err)
			return nil
		}

		documents = append(documents, doc)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory %s: %w", dataDir, err)
	}

	return documents, nil
}
