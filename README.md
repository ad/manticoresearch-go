# Manticore Search Tester

A Go application for testing and comparing different search approaches using Manticore Search, including full-text search, vector search, and hybrid search methods.

## Features

- **HTTP API**: RESTful API for search operations
- **Document Processing**: Parse markdown files and extract structured data
- **Multiple Search Modes**:
  - Basic text search (simple string matching)
  - Full-text search using Manticore Search with BM25 scoring
  - Vector search using TF-IDF vectors and cosine similarity
  - Hybrid search combining full-text and vector approaches
- **TF-IDF Vectorization**: Custom implementation for semantic search
- **Manticore Integration**: HTTP JSON API interface with Manticore Search
- **Docker Support**: Easy setup with Docker Compose

## Prerequisites

- Go 1.23 or higher
- Docker and Docker Compose (for Manticore Search)
- Make (optional, for using Makefile commands)

## Quick Start

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd manticore-search-tester
   ```

2. **Start Manticore Search**:
   ```bash
   make docker-up
   # or
   docker-compose up -d
   ```

3. **Build and run the application**:
   ```bash
   make run
   # or
   make build && ./bin/manticore-search-tester
   ```

4. **Test the API**:
   ```bash
   curl "http://localhost:8080/api/search?query=сайт&mode=basic"
   curl "http://localhost:8080/api/status"
   ```

## Project Structure

```
.
├── cmd/
│   └── server/          # Application entry point
│       └── main.go
├── internal/            # Private application code
│   ├── document/        # Document parsing and processing
│   ├── handlers/        # HTTP request handlers
│   ├── manticore/       # Manticore Search client
│   ├── models/          # Data models and types
│   ├── search/          # Search engine implementations
│   └── vectorizer/      # TF-IDF vectorization
├── pkg/                 # Public API types
│   └── api/
├── data/                # Sample markdown documents
├── bin/                 # Built binaries
├── docker-compose.yml   # Docker setup for Manticore Search
├── Dockerfile           # Application container
├── Makefile            # Build and development commands
├── .air.toml           # Hot reload configuration
└── go.mod              # Go module dependencies
```

## API Endpoints

### Search API - `GET /api/search`
Search across indexed documents with different modes.

**Parameters:**
- `query` (required): Search query string
- `mode` (optional): `basic`, `fulltext`, `vector`, or `hybrid` (default: `basic`)
- `page` (optional): Page number (default: 1)
- `limit` (optional): Results per page, 1-100 (default: 10)

**Example:**
```bash
curl "http://localhost:8080/api/search?query=добавить блок&mode=fulltext&page=1&limit=5"
```

### Status API - `GET /api/status`
Get service health and status information.

**Example:**
```bash
curl "http://localhost:8080/api/status"
```

### Reindex API - `POST /api/reindex`
Manually trigger document reindexing.

**Example:**
```bash
curl -X POST "http://localhost:8080/api/reindex"
```

## Development Commands

### Using Makefile

```bash
# Build the application
make build

# Run the application
make run

# Run in development mode with auto-restart
make dev

# Start full development environment (Docker + dev server)
make dev-full

# Run tests
make test

# Test API endpoints
make test-api

# Docker commands
make docker-up      # Start Manticore Search
make docker-down    # Stop Manticore Search
make docker-logs    # View Docker logs

# Code quality
make fmt           # Format code
make lint          # Lint code (requires golangci-lint)

# Build for multiple platforms
make build-all

# Create release archives
make release

# Install development tools
make install-tools

# Show all available commands
make help
```

### Manual Commands

```bash
# Install dependencies
go mod download && go mod tidy

# Build
go build -o bin/manticore-search-tester ./cmd/server

# Run
./bin/manticore-search-tester

# Test API
./bin/manticore-search-tester test-api
```

## Search Modes

### 1. Basic Text Search (`basic`)
Simple string matching with scoring based on:
- Title matches (higher weight)
- Content matches
- Word frequency
- Document length normalization

### 2. Full-Text Search (`fulltext`)
Uses Manticore Search's built-in full-text capabilities:
- BM25 scoring algorithm
- Advanced query syntax support
- Optimized for large document collections

### 3. Vector Search (`vector`)
Semantic search using TF-IDF vectors:
- Custom TF-IDF implementation
- Cosine similarity scoring
- Handles synonyms and related terms better

### 4. Hybrid Search (`hybrid`)
Combines full-text and vector search:
- Weighted combination (70% full-text, 30% vector)
- Re-ranking of combined results
- Best of both approaches

## Configuration

### Environment Variables

#### Basic Configuration
- `MANTICORE_HOST`: Manticore Search host (default: `localhost:9308`)
- `DATA_DIR`: Directory containing markdown files (default: `./data`)
- `PORT`: HTTP server port (default: `8080`)

#### Manticore HTTP Client Configuration
- `MANTICORE_HTTP_TIMEOUT`: HTTP request timeout (default: `60s`)
- `MANTICORE_HTTP_MAX_IDLE_CONNS`: Maximum idle connections (default: `20`)
- `MANTICORE_HTTP_MAX_IDLE_CONNS_PER_HOST`: Maximum idle connections per host (default: `10`)
- `MANTICORE_HTTP_IDLE_CONN_TIMEOUT`: Idle connection timeout (default: `90s`)

#### Retry Configuration
- `MANTICORE_HTTP_RETRY_MAX_ATTEMPTS`: Maximum retry attempts (default: `5`)
- `MANTICORE_HTTP_RETRY_BASE_DELAY`: Base retry delay (default: `500ms`)
- `MANTICORE_HTTP_RETRY_MAX_DELAY`: Maximum retry delay (default: `30s`)
- `MANTICORE_HTTP_RETRY_JITTER_PERCENT`: Retry jitter percentage (default: `0.1`)

#### Circuit Breaker Configuration
- `MANTICORE_HTTP_CB_FAILURE_THRESHOLD`: Circuit breaker failure threshold (default: `5`)
- `MANTICORE_HTTP_CB_RECOVERY_TIMEOUT`: Circuit breaker recovery timeout (default: `30s`)
- `MANTICORE_HTTP_CB_HALF_OPEN_MAX_CALLS`: Half-open state max calls (default: `3`)

### Document Format

Documents should be markdown files with this structure:

```markdown
# Document Title

**URL:** https://example.com/document-url

Document content goes here...
```

## Docker Support

### Using Docker Compose

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

### Building Docker Image

```bash
docker build -t manticore-search-tester .
```

### GPU Acceleration (Optional)

For improved Auto Embeddings performance, you can enable GPU acceleration:

**Prerequisites:**
- NVIDIA GPU with CUDA support
- NVIDIA Docker runtime installed
- `nvidia-container-toolkit` or `nvidia-docker2`

**Automatic GPU Setup:**
```bash
# Auto-detect and enable GPU if available
./scripts/gpu-setup.sh

# Force GPU mode
./scripts/gpu-setup.sh --force-gpu

# Check GPU requirements
./scripts/gpu-setup.sh --check
```

**Manual GPU Setup:**
```bash
# With GPU acceleration
docker-compose -f docker-compose.yml -f docker-compose.gpu.yml up

# Without GPU (default)
docker-compose up
```

**Performance Benefits:**
- 2-10x faster vector generation for Auto Embeddings
- Reduced CPU load during document indexing  
- Better performance for large document sets (500+ documents)

## Development

### Hot Reload Development

Install Air for hot reload during development:

```bash
make install-tools  # Installs air and other dev tools
make dev            # Start with hot reload
```

### Adding New Documents

1. Place markdown files in the `./data` directory
2. Call the reindex API: `curl -X POST "http://localhost:8080/api/reindex"`

### Extending Functionality

The modular architecture makes it easy to extend:

- **Add new search modes**: Extend `internal/search/engine.go`
- **Modify scoring**: Update scoring algorithms in search implementations
- **Add new vectorizers**: Implement new vectorization methods in `internal/vectorizer/`
- **Add new document formats**: Extend `internal/document/parser.go`

## HTTP Client Implementation

This application uses a custom HTTP JSON API client implementation for Manticore Search. This approach provides several benefits over using third-party libraries:

### Benefits of HTTP Client
- **Better Control**: Direct control over HTTP requests, timeouts, and error handling
- **Reduced Dependencies**: No longer depends on infrequently updated third-party libraries
- **Simplified Architecture**: Removed factory pattern complexity, direct client creation
- **Enhanced Resilience**: Built-in circuit breaker and retry mechanisms
- **Improved Performance**: Optimized connection pooling and bulk operations
- **Better Debugging**: Comprehensive logging of all HTTP operations

### Key Features
- **Circuit Breaker Pattern**: Automatic failure detection and recovery
- **Exponential Backoff**: Smart retry logic with jitter for network resilience
- **Connection Pooling**: Efficient HTTP connection management
- **Bulk Operations**: Optimized batch processing using NDJSON format
- **Comprehensive Logging**: Detailed request/response logging for troubleshooting

### Implementation Features
The HTTP client provides robust and efficient operations:
- All search modes work reliably with comprehensive error handling
- Optimized document indexing with intelligent bulk operations
- Consistent API responses with detailed logging
- Built-in resilience patterns (circuit breaker, exponential backoff retry)
- Connection pooling and HTTP keep-alive for optimal performance

## Architecture

### Package Structure

- **`cmd/server`**: Application entry point and initialization
- **`internal/handlers`**: HTTP request handlers and routing
- **`internal/search`**: Search engine implementations
- **`internal/document`**: Document parsing and processing
- **`internal/manticore`**: Manticore Search client and operations
- **`internal/vectorizer`**: TF-IDF vectorization implementation
- **`internal/models`**: Shared data models and types
- **`pkg/api`**: Public API response types

### Data Flow

```
HTTP Request → Handlers → Search Engine → Manticore/Vector Search → JSON Response
                    ↓
Markdown Files → Document Parser → TF-IDF Vectorizer → Manticore Indexing
```

## Performance Considerations

- **Indexing**: Batch operations for better performance
- **Memory**: TF-IDF vectors are kept in memory for fast access
- **Caching**: Connection pooling and prepared statements
- **Scaling**: Manticore Search handles large document collections efficiently
- **API**: CORS support for web applications

## Troubleshooting

### Common Issues

1. **Connection refused**: Ensure Manticore Search is running (`make docker-up`)
2. **No documents found**: Check the `./data` directory exists and contains `.md` files
3. **Build errors**: Run `go mod download && go mod tidy` to install dependencies
4. **Port conflicts**: Change the `PORT` environment variable

### Debugging

Enable verbose logging:
```bash
export MANTICORE_DEBUG=1
export SEARCH_DEBUG=1
```

Check service status:
```bash
curl "http://localhost:8080/api/status"
```

### HTTP Client Troubleshooting

#### Connection Issues
If you're experiencing connection problems with Manticore Search:

1. **Check Manticore is running on the correct port**:
   ```bash
   docker-compose ps
   curl -X GET "http://localhost:9308/"
   ```

2. **Verify HTTP JSON API is enabled**:
   The application uses Manticore's HTTP JSON API on port 9308 (not the MySQL protocol on port 9306).

3. **Test API endpoints manually**:
   ```bash
   # Health check
   curl -X GET "http://localhost:9308/"
   
   # Test search endpoint
   curl -X POST "http://localhost:9308/search" \
     -H "Content-Type: application/json" \
     -d '{"index": "documents", "query": {"match_all": {}}}'
   ```

#### Circuit Breaker Issues
If the circuit breaker is frequently opening:

1. **Check failure threshold**: Lower `MANTICORE_HTTP_CB_FAILURE_THRESHOLD` if needed
2. **Increase recovery timeout**: Set `MANTICORE_HTTP_CB_RECOVERY_TIMEOUT` to a higher value
3. **Monitor logs**: Look for repeated connection failures

#### Retry Configuration
If requests are timing out or failing:

1. **Increase timeout**: Set `MANTICORE_HTTP_TIMEOUT` to a higher value
2. **Adjust retry attempts**: Increase `MANTICORE_HTTP_RETRY_MAX_ATTEMPTS`
3. **Modify retry delays**: Adjust `MANTICORE_HTTP_RETRY_BASE_DELAY` and `MANTICORE_HTTP_RETRY_MAX_DELAY`

#### Performance Tuning
For better performance:

1. **Connection pooling**: Increase `MANTICORE_HTTP_MAX_IDLE_CONNS` and `MANTICORE_HTTP_MAX_IDLE_CONNS_PER_HOST`
2. **Keep-alive**: Increase `MANTICORE_HTTP_IDLE_CONN_TIMEOUT`
3. **Bulk operations**: The client automatically uses bulk operations for better throughput

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Submit a pull request

## License

This project is provided as-is for educational and testing purposes.