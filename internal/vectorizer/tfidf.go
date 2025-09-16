package vectorizer

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ad/manticoresearch-go/internal/models"
)

// TFIDFVectorizer implements a simple TF-IDF vectorization
type TFIDFVectorizer struct {
	vocabulary map[string]int // word -> index mapping
	idf        []float64      // inverse document frequency for each word
	documents  []string       // preprocessed documents for IDF calculation
}

// NewTFIDFVectorizer creates a new TF-IDF vectorizer
func NewTFIDFVectorizer() *TFIDFVectorizer {
	return &TFIDFVectorizer{
		vocabulary: make(map[string]int),
		documents:  make([]string, 0),
	}
}

// preprocessText cleans and tokenizes text
func (v *TFIDFVectorizer) preprocessText(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Remove punctuation and special characters, keep only letters and numbers
	reg := regexp.MustCompile(`[^a-zA-Zа-яА-Я0-9\s]+`)
	text = reg.ReplaceAllString(text, " ")

	// Split into words and filter out short words
	words := strings.Fields(text)
	var filteredWords []string

	for _, word := range words {
		// Keep words that are at least 2 characters long
		if len(word) >= 2 {
			filteredWords = append(filteredWords, word)
		}
	}

	return filteredWords
}

// FitTransform builds vocabulary and calculates IDF from documents, then transforms them
func (v *TFIDFVectorizer) FitTransform(documents []*models.Document) [][]float64 {
	// Step 1: Build vocabulary from all documents
	wordCounts := make(map[string]int)

	// Preprocess all documents and collect words
	for _, doc := range documents {
		// Combine title and content for vectorization
		fullText := doc.Title + " " + doc.Content
		words := v.preprocessText(fullText)
		v.documents = append(v.documents, fullText)

		// Count unique words per document for IDF calculation
		uniqueWords := make(map[string]bool)
		for _, word := range words {
			uniqueWords[word] = true
		}

		// Increment document frequency for each unique word
		for word := range uniqueWords {
			wordCounts[word]++
		}
	}

	// Build vocabulary (only keep words that appear in at least 2 documents)
	var vocabWords []string
	for word, count := range wordCounts {
		if count >= 2 {
			vocabWords = append(vocabWords, word)
		}
	}

	// Sort vocabulary for consistent indexing
	sort.Strings(vocabWords)

	// Create word -> index mapping
	for i, word := range vocabWords {
		v.vocabulary[word] = i
	}

	// Step 2: Calculate IDF for each word
	v.idf = make([]float64, len(v.vocabulary))
	totalDocs := float64(len(documents))

	for word, index := range v.vocabulary {
		docFreq := float64(wordCounts[word])
		v.idf[index] = math.Log(totalDocs / docFreq)
	}

	// Step 3: Transform documents to TF-IDF vectors
	vectors := make([][]float64, len(documents))
	for i, doc := range documents {
		vectors[i] = v.transformDocument(doc.Title + " " + doc.Content)
	}

	return vectors
}

// transformDocument converts a single document to TF-IDF vector
func (v *TFIDFVectorizer) transformDocument(text string) []float64 {
	words := v.preprocessText(text)
	vector := make([]float64, len(v.vocabulary))

	// Count term frequencies
	termFreq := make(map[string]int)
	for _, word := range words {
		termFreq[word]++
	}

	// Calculate TF-IDF for each word in vocabulary
	totalWords := float64(len(words))
	for word, index := range v.vocabulary {
		tf := float64(termFreq[word]) / totalWords
		if tf > 0 {
			vector[index] = tf * v.idf[index]
		}
	}

	// Normalize vector (L2 normalization)
	norm := 0.0
	for _, val := range vector {
		norm += val * val
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector
}

// TransformQuery converts a query string to TF-IDF vector
func (v *TFIDFVectorizer) TransformQuery(query string) []float64 {
	return v.transformDocument(query)
}

// CosineSimilarity calculates cosine similarity between two vectors
func CosineSimilarity(vec1, vec2 []float64) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct, norm1, norm2 float64

	for i := 0; i < len(vec1); i++ {
		dotProduct += vec1[i] * vec2[i]
		norm1 += vec1[i] * vec1[i]
		norm2 += vec2[i] * vec2[i]
	}

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

// VectorSearchResult represents a document with its similarity score
type VectorSearchResult struct {
	Document   *models.Document
	Similarity float64
}

// VectorSearch performs semantic search using TF-IDF vectors
func VectorSearch(query string, documents []*models.Document, vectors [][]float64, vectorizer *TFIDFVectorizer, limit int) []VectorSearchResult {
	queryVector := vectorizer.TransformQuery(query)

	var results []VectorSearchResult

	for i, doc := range documents {
		similarity := CosineSimilarity(queryVector, vectors[i])
		if similarity > 0 {
			results = append(results, VectorSearchResult{
				Document:   doc,
				Similarity: similarity,
			})
		}
	}

	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results
}
