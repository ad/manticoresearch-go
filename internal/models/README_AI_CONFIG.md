# AI Search Configuration

This document describes how to configure AI search functionality in the Manticore Search Tester application.

## Environment Variables

The following environment variables can be used to configure AI search:

| Variable | Description | Default Value | Example |
|----------|-------------|---------------|---------|
| `MANTICORE_AI_MODEL` | The embedding model to use for AI search | `sentence-transformers/all-MiniLM-L6-v2` | `custom-model/bert-base` |
| `MANTICORE_AI_ENABLED` | Enable or disable AI search functionality | `true` | `false` |
| `MANTICORE_AI_TIMEOUT` | Timeout for AI search requests | `30s` | `45s` |

## Usage Example

```go
package main

import (
    "log"
    "github.com/ad/manticoresearch-go/internal/models"
)

func main() {
    // Initialize AI search configuration
    config, err := models.InitializeAISearchConfig()
    if err != nil {
        log.Fatalf("Failed to initialize AI search config: %v", err)
    }

    // Check if AI search is enabled
    if config.Enabled {
        log.Printf("AI search is enabled with model: %s", config.Model)
    } else {
        log.Println("AI search is disabled")
    }

    // Get AI search status for monitoring
    status := models.GetAISearchStatus(config)
    log.Printf("AI search status: %+v", status)
}
```

## Configuration Validation

The configuration system includes comprehensive validation:

- **Model Name Validation**: Prevents path traversal and injection attacks
- **Timeout Validation**: Ensures positive timeout values
- **Boolean Validation**: Validates enabled/disabled flags
- **Fallback Logic**: Falls back to default configuration on errors

## Error Handling

The configuration system implements graceful error handling:

1. **Invalid Environment Variables**: Logs warnings and falls back to defaults
2. **Missing Configuration**: Uses sensible default values
3. **Security Validation**: Rejects potentially dangerous model names
4. **Type Validation**: Ensures proper data types for all configuration values

## Default Configuration

When no environment variables are set, the system uses these defaults:

```go
&AISearchConfig{
    Model:   "sentence-transformers/all-MiniLM-L6-v2",
    Enabled: true,
    Timeout: 30 * time.Second,
}
```

## Security Considerations

The configuration system includes security measures:

- Model names are validated to prevent path traversal attacks
- Dangerous characters and patterns are rejected
- Input sanitization prevents injection attacks
- Timeout limits prevent resource exhaustion

## Integration with Application

The AI search configuration integrates with the existing application architecture:

1. Load configuration during application startup
2. Pass configuration to search engine components
3. Include AI search status in health checks
4. Log configuration details for debugging

## Testing

The configuration system includes comprehensive tests:

- Unit tests for all configuration functions
- Validation tests for security measures
- Error handling tests for edge cases
- Integration tests with environment variables

Run tests with:
```bash
go test ./internal/models -v
```