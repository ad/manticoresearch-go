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
- **Manticore Integration**: Direct SQL interface with Manticore Search
- **Docker Support**: Easy setup with Docker Compose

## Prerequisites

- Go 1.21 or higher
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

- `MANTICORE_HOST`: Manticore Search host (default: `localhost:9306`)
- `DATA_DIR`: Directory containing markdown files (default: `./data`)
- `PORT`: HTTP server port (default: `8080`)

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
3. **Build errors**: Run `make deps` to install dependencies
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

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Submit a pull request

## License

This project is licensed under the MIT License - see the LICENSE file for details.