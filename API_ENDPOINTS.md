# API Endpoints Documentation

This document describes the HTTP API endpoints implemented for the Manticore Search Tester.

## Base URL
When running locally: `http://localhost:8080`

## Endpoints

### 1. Search API - `GET /api/search`

Performs search across indexed documents using different search modes.

**Query Parameters:**
- `query` (required): Search query string
- `mode` (optional): Search mode - `basic`, `fulltext`, `vector`, or `hybrid` (default: `basic`)
- `page` (optional): Page number for pagination (default: 1, min: 1)
- `limit` (optional): Number of results per page (default: 10, min: 1, max: 100)

**Example Requests:**
```bash
# Basic text search
curl "http://localhost:8080/api/search?query=сайт&mode=basic&page=1&limit=5"

# Full-text search with Manticore
curl "http://localhost:8080/api/search?query=добавить блок&mode=fulltext"

# Vector semantic search
curl "http://localhost:8080/api/search?query=создать форму&mode=vector"

# Hybrid search (combines full-text and vector)
curl "http://localhost:8080/api/search?query=настроить дизайн&mode=hybrid"
```

**Response Format:**
```json
{
  "success": true,
  "data": {
    "documents": [
      {
        "document": {
          "id": 1,
          "title": "Document Title",
          "url": "https://example.com/doc",
          "content": "Document content..."
        },
        "score": 8.5
      }
    ],
    "total": 25,
    "page": 1,
    "mode": "basic"
  }
}
```

**Error Response:**
```json
{
  "success": false,
  "error": "Query parameter is required"
}
```

### 2. Status API - `GET /api/status`

Returns the current status of the search service and its components.

**Example Request:**
```bash
curl "http://localhost:8080/api/status"
```

**Response Format:**
```json
{
  "success": true,
  "data": {
    "status": "ok",
    "manticore_healthy": true,
    "documents_loaded": 150,
    "vectorizer_ready": true
  }
}
```

**Response Fields:**
- `status`: Overall service status (`ok` or `error`)
- `manticore_healthy`: Whether Manticore Search is connected and healthy
- `documents_loaded`: Number of documents currently indexed
- `vectorizer_ready`: Whether the TF-IDF vectorizer is initialized

### 3. Reindex API - `POST /api/reindex`

Manually triggers reindexing of all documents from the data directory.

**Example Request:**
```bash
curl -X POST "http://localhost:8080/api/reindex"
```

**Response Format:**
```json
{
  "success": true,
  "data": {
    "message": "Reindexing completed successfully",
    "documents_count": 150,
    "indexing_time": "2.5s"
  }
}
```

**Error Response (when Manticore is unavailable):**
```json
{
  "success": false,
  "error": "Manticore Search is not available"
}
```

## Error Handling

All endpoints return appropriate HTTP status codes:

- `200 OK`: Successful request
- `400 Bad Request`: Invalid parameters or missing required fields
- `405 Method Not Allowed`: Wrong HTTP method used
- `500 Internal Server Error`: Server-side error during processing
- `503 Service Unavailable`: Required services (like Manticore) are not available

## CORS Support

All endpoints include CORS headers to allow cross-origin requests:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type`

## Search Modes

1. **Basic**: Simple text matching against title and content
2. **Full-text**: Uses Manticore's full-text search with BM25 scoring
3. **Vector**: Semantic search using TF-IDF vectors and cosine similarity
4. **Hybrid**: Combines full-text (70%) and vector (30%) search results

## Testing

You can test the API endpoints using the built-in test function:

```bash
./manticore-search-tester test-api
```

This will run basic tests against all endpoints and show the responses.