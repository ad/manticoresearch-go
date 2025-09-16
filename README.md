# Manticore Search Tester

A comprehensive testing service for Manticore Search demonstrating both full-text and vector search capabilities.

## Quick Start

1. Ensure Docker and Docker Compose are installed
2. Clone this repository
3. Run the application:

```bash
docker-compose up --build
```

4. Access the web interface at http://localhost:8080

## Services

- **Web Service**: http://localhost:8080 - Go-based web service and UI
- **Manticore Search**: localhost:9306 (SQL), localhost:9308 (HTTP)

## Development

To run locally without Docker:

```bash
go mod download
go run main.go
```

Make sure Manticore Search is running separately on localhost:9306.

## Environment Variables

- `MANTICORE_HOST` - Manticore connection string (default: localhost:9306)
- `PORT` - HTTP server port (default: 8080)
- `DATA_DIR` - Directory containing markdown files (default: ./data)