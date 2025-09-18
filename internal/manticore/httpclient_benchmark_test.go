package manticore

import (
	"fmt"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ad/manticoresearch-go/internal/models"
)

// Benchmark tests for HTTP client performance
// These benchmarks require a real Manticore instance
// Set MANTICORE_BENCHMARK_TESTS=1 to run benchmarks

func skipBenchmarkIfNoManticore(b *testing.B) {
	if os.Getenv("MANTICORE_BENCHMARK_TESTS") != "1" {
		b.Skip("Skipping benchmark test. Set MANTICORE_BENCHMARK_TESTS=1 to run.")
	}
}

func createBenchmarkClient(b *testing.B) ClientInterface {
	config := DefaultHTTPClientConfig(getManticoreURL())
	// Optimize for benchmarking
	config.Timeout = 60 * time.Second
	config.RetryConfig.MaxAttempts = 1                  // Disable retries for consistent timing
	config.CircuitBreakerConfig.FailureThreshold = 1000 // Disable circuit breaker

	client := NewHTTPClient(config)

	// Wait for Manticore to be ready
	err := client.WaitForReady(30 * time.Second)
	if err != nil {
		b.Fatalf("Failed to connect to Manticore at %s: %v", getManticoreURL(), err)
	}

	// Setup schema
	err = client.CreateSchema()
	if err != nil {
		b.Fatalf("Failed to create schema: %v", err)
	}

	return client
}

// Benchmark single document indexing
func BenchmarkIndexSingleDocument(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	// Create test document
	doc := &models.Document{
		ID:      1,
		Title:   "Benchmark Test Document",
		Content: "This is a benchmark test document with some content to index",
		URL:     "http://example.com/benchmark",
	}
	vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		doc.ID = i + 1 // Use unique IDs
		err := client.IndexDocument(doc, vector)
		if err != nil {
			b.Fatalf("IndexDocument failed: %v", err)
		}
	}
}

func BenchmarkIndexSingleDocumentNoVector(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	doc := &models.Document{
		ID:      1,
		Title:   "Benchmark Test Document",
		Content: "This is a benchmark test document with some content to index",
		URL:     "http://example.com/benchmark",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		doc.ID = i + 1
		err := client.IndexDocument(doc, nil)
		if err != nil {
			b.Fatalf("IndexDocument failed: %v", err)
		}
	}
}

// Benchmark bulk document indexing with different batch sizes
func BenchmarkIndexBulkDocuments10(b *testing.B) {
	benchmarkIndexBulkDocuments(b, 10)
}

func BenchmarkIndexBulkDocuments50(b *testing.B) {
	benchmarkIndexBulkDocuments(b, 50)
}

func BenchmarkIndexBulkDocuments100(b *testing.B) {
	benchmarkIndexBulkDocuments(b, 100)
}

func BenchmarkIndexBulkDocuments500(b *testing.B) {
	benchmarkIndexBulkDocuments(b, 500)
}

func benchmarkIndexBulkDocuments(b *testing.B, batchSize int) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	// Create test documents
	documents := make([]*models.Document, batchSize)
	vectors := make([][]float64, batchSize)

	for i := 0; i < batchSize; i++ {
		documents[i] = &models.Document{
			ID:      i + 1,
			Title:   fmt.Sprintf("Benchmark Document %d", i+1),
			Content: fmt.Sprintf("This is benchmark content for document %d with additional text to make it realistic", i+1),
			URL:     fmt.Sprintf("http://example.com/benchmark-%d", i+1),
		}
		vectors[i] = []float64{
			float64(i) * 0.001,
			float64(i) * 0.002,
			float64(i) * 0.003,
			float64(i) * 0.004,
			float64(i) * 0.005,
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Update IDs to avoid conflicts
		for j := range documents {
			documents[j].ID = i*batchSize + j + 1
		}

		err := client.IndexDocuments(documents, vectors)
		if err != nil {
			b.Fatalf("IndexDocuments failed: %v", err)
		}
	}

	// Report custom metrics
	docsPerOp := float64(batchSize)
	totalDocs := float64(b.N) * docsPerOp
	elapsed := time.Duration(b.Elapsed().Nanoseconds())
	docsPerSecond := totalDocs / elapsed.Seconds()

	b.ReportMetric(docsPerSecond, "docs/sec")
	b.ReportMetric(docsPerOp, "docs/op")
}

// Benchmark search operations
func BenchmarkSearchBasic(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	// Index some test data first
	setupBenchmarkData(b, client, 1000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match": map[string]interface{}{
				"*": "benchmark",
			},
		},
		Limit: 10,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.SearchWithRequest(request)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkSearchFullText(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 1000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"query_string": "benchmark AND content",
		},
		Limit: 10,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.SearchWithRequest(request)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkSearchMatchAll(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 1000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		Limit: 50,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.SearchWithRequest(request)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

// Benchmark search with different result sizes
func BenchmarkSearchLimit10(b *testing.B) {
	benchmarkSearchWithLimit(b, 10)
}

func BenchmarkSearchLimit50(b *testing.B) {
	benchmarkSearchWithLimit(b, 50)
}

func BenchmarkSearchLimit100(b *testing.B) {
	benchmarkSearchWithLimit(b, 100)
}

func benchmarkSearchWithLimit(b *testing.B, limit int32) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 2000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		Limit: limit,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		response, err := client.SearchWithRequest(request)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}

		// Verify we got results
		if len(response.Hits.Hits) == 0 {
			b.Fatal("Expected search results but got none")
		}
	}

	b.ReportMetric(float64(limit), "results/op")
}

// Benchmark schema operations
func BenchmarkCreateSchema(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	config := DefaultHTTPClientConfig(getManticoreURL())
	config.RetryConfig.MaxAttempts = 1
	config.CircuitBreakerConfig.FailureThreshold = 1000

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		client := NewHTTPClient(config)

		err := client.WaitForReady(10 * time.Second)
		if err != nil {
			b.Fatalf("Failed to connect: %v", err)
		}

		err = client.CreateSchema()
		if err != nil {
			b.Fatalf("CreateSchema failed: %v", err)
		}

		client.Close()
	}
}

func BenchmarkTruncateTables(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := client.TruncateTables()
		if err != nil {
			b.Fatalf("TruncateTables failed: %v", err)
		}
	}
}

// Benchmark concurrent operations
func BenchmarkConcurrentSearch(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 1000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match": map[string]interface{}{
				"*": "benchmark",
			},
		},
		Limit: 10,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := client.SearchWithRequest(request)
			if err != nil {
				b.Fatalf("Concurrent search failed: %v", err)
			}
		}
	})
}

func BenchmarkConcurrentIndexing(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		docID := 1
		for pb.Next() {
			doc := &models.Document{
				ID:      docID,
				Title:   fmt.Sprintf("Concurrent Document %d", docID),
				Content: fmt.Sprintf("Concurrent content %d", docID),
				URL:     fmt.Sprintf("http://example.com/concurrent-%d", docID),
			}
			vector := []float64{float64(docID) * 0.001, float64(docID) * 0.002}

			err := client.IndexDocument(doc, vector)
			if err != nil {
				b.Fatalf("Concurrent indexing failed: %v", err)
			}
			docID++
		}
	})
}

// Memory usage benchmarks
func BenchmarkMemoryUsageIndexing(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	// Force GC before starting
	runtime.GC()

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		doc := &models.Document{
			ID:      i + 1,
			Title:   fmt.Sprintf("Memory Test Document %d", i+1),
			Content: fmt.Sprintf("This is memory test content for document %d with additional text", i+1),
			URL:     fmt.Sprintf("http://example.com/memory-%d", i+1),
		}
		vector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}

		err := client.IndexDocument(doc, vector)
		if err != nil {
			b.Fatalf("IndexDocument failed: %v", err)
		}
	}

	runtime.ReadMemStats(&m2)

	// Report memory metrics
	allocsPerOp := float64(m2.Mallocs-m1.Mallocs) / float64(b.N)
	bytesPerOp := float64(m2.TotalAlloc-m1.TotalAlloc) / float64(b.N)

	b.ReportMetric(allocsPerOp, "allocs/op")
	b.ReportMetric(bytesPerOp, "bytes/op")
}

func BenchmarkMemoryUsageSearch(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 1000)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
		Limit: 50,
	}

	// Force GC before starting
	runtime.GC()

	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := client.SearchWithRequest(request)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}

	runtime.ReadMemStats(&m2)

	allocsPerOp := float64(m2.Mallocs-m1.Mallocs) / float64(b.N)
	bytesPerOp := float64(m2.TotalAlloc-m1.TotalAlloc) / float64(b.N)

	b.ReportMetric(allocsPerOp, "allocs/op")
	b.ReportMetric(bytesPerOp, "bytes/op")
}

// Latency benchmarks with different network conditions
func BenchmarkLatencyLocal(b *testing.B) {
	skipBenchmarkIfNoManticore(b)

	client := createBenchmarkClient(b)
	defer client.Close()

	setupBenchmarkData(b, client, 100)

	request := SearchRequest{
		Index: "documents",
		Query: map[string]interface{}{
			"match": map[string]interface{}{
				"*": "benchmark",
			},
		},
		Limit: 1,
	}

	latencies := make([]time.Duration, b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, err := client.SearchWithRequest(request)
		latencies[i] = time.Since(start)

		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}

	// Calculate latency statistics
	var total time.Duration
	min := latencies[0]
	max := latencies[0]

	for _, lat := range latencies {
		total += lat
		if lat < min {
			min = lat
		}
		if lat > max {
			max = lat
		}
	}

	avg := total / time.Duration(len(latencies))

	b.ReportMetric(float64(avg.Nanoseconds())/1e6, "avg_latency_ms")
	b.ReportMetric(float64(min.Nanoseconds())/1e6, "min_latency_ms")
	b.ReportMetric(float64(max.Nanoseconds())/1e6, "max_latency_ms")
}

// Helper function to setup benchmark data
func setupBenchmarkData(b *testing.B, client ClientInterface, docCount int) {
	b.Helper()

	// Clear existing data
	err := client.TruncateTables()
	if err != nil {
		b.Fatalf("Failed to truncate tables: %v", err)
	}

	// Create test documents
	documents := make([]*models.Document, docCount)
	vectors := make([][]float64, docCount)

	for i := 0; i < docCount; i++ {
		documents[i] = &models.Document{
			ID:      i + 1,
			Title:   fmt.Sprintf("Benchmark Setup Document %d", i+1),
			Content: fmt.Sprintf("This is benchmark setup content for document %d with searchable text and benchmark keywords", i+1),
			URL:     fmt.Sprintf("http://example.com/setup-%d", i+1),
		}
		vectors[i] = []float64{
			float64(i) * 0.01,
			float64(i) * 0.02,
			float64(i) * 0.03,
		}
	}

	// Index in batches for better performance
	batchSize := 100
	for i := 0; i < len(documents); i += batchSize {
		end := i + batchSize
		if end > len(documents) {
			end = len(documents)
		}

		err := client.IndexDocuments(documents[i:end], vectors[i:end])
		if err != nil {
			b.Fatalf("Failed to setup benchmark data: %v", err)
		}
	}

	// Wait a moment for indexing to complete
	time.Sleep(1 * time.Second)
}
