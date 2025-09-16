package document

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ad/manticoresearch-go/internal/models"
)

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

	// Validate required fields
	if err := validateDocument(doc); err != nil {
		return nil, fmt.Errorf("validation failed for %s: %w", filePath, err)
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
	var docID int = 1

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

		doc.ID = docID
		docID++
		documents = append(documents, doc)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan directory %s: %w", dataDir, err)
	}

	return documents, nil
}
