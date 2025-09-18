# Manticore HTTP Client Architecture

Этот пакет содержит HTTP-клиент для Manticore Search, разделенный на логические модули для лучшей организации кода.

## Структура файлов

### Основные файлы

- **`httpclient_types.go`** - Интерфейсы, типы данных и конфигурация
  - `ClientInterface` интерфейс
  - Конфигурационные структуры (`HTTPClientConfig`, `BulkConfig`)
  - JSON API типы запросов и ответов
  - `SearchResultProcessor` для обработки результатов поиска

- **`httpclient_core.go`** - Основная структура клиента и базовые методы
  - `manticoreHTTPClient` структура
  - `NewHTTPClient()` конструктор
  - Методы управления соединением (`WaitForReady`, `HealthCheck`, `Close`)
  - Интеграция с мониторингом и метриками

### Функциональные модули

- **`httpclient_schema.go`** - Операции со схемой базы данных
  - `executeSQL()` - выполнение SQL команд
  - `CreateSchema()` - создание таблиц
  - `ResetDatabase()` - сброс базы данных
  - `TruncateTables()` - очистка таблиц

- **`httpclient_indexing.go`** - Операции индексирования документов
  - `IndexDocument()` - индексирование одного документа
  - `IndexDocuments()` - массовое индексирование
  - `indexDocumentFullText()` - индексирование в полнотекстовой таблице
  - `indexDocumentVector()` - индексирование в векторной таблице

- **`httpclient_bulk.go`** - Массовые операции
  - `singleBulkIndex()` - одиночная массовая операция
  - `batchedBulkIndex()` - пакетная обработка
  - `streamingBulkIndex()` - потоковая обработка больших объемов
  - `bulkIndexFullText()` / `bulkIndexVectors()` - массовое индексирование
  - Воркеры для параллельной обработки

- **`httpclient_search.go`** - Операции поиска
  - `SearchWithRequest()` - основной метод поиска
  - `GetAllDocuments()` - получение всех документов
  - Создание различных типов поисковых запросов
  - Конвертация ответов в внутренние модели
  - Векторный поиск и вычисление сходства
  - `SearchResultProcessor` методы для обработки результатов

### Вспомогательные файлы

- **`client_config.go`** - Конфигурация и создание клиента
- **`search_adapter.go`** - Адаптер для унификации поиска
- **`monitoring.go`** - Система мониторинга и метрик
- **`circuit_breaker.go`** - Circuit breaker паттерн
- **`retry.go`** - Система повторных попыток
- **`errors.go`** - Обработка ошибок

## Основные возможности

### Client Configuration
Система использует HTTP JSON API клиент для взаимодействия с Manticore Search.

Настройки HTTP клиента:
```bash
export MANTICORE_HTTP_TIMEOUT=60s
export MANTICORE_HTTP_MAX_IDLE_CONNS=20
export MANTICORE_HTTP_RETRY_MAX_ATTEMPTS=5
```

### Мониторинг и метрики
- Сбор метрик производительности
- Отслеживание состояния circuit breaker
- Логирование операций с настраиваемыми уровнями
- Периодическая отчетность

### Устойчивость к сбоям
- Circuit breaker для защиты от каскадных сбоев
- Система повторных попыток с экспоненциальной задержкой
- Таймауты и управление соединениями
- Graceful degradation при сбоях

### Производительность
- Массовые операции с оптимизацией размера пакетов
- Потоковая обработка для больших объемов данных
- Параллельная обработка пакетов
- Переиспользование соединений

## Использование

```go
// Создание клиента из переменных окружения
client, err := manticore.NewClientFromEnvironment()
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Или создание с кастомной конфигурацией
config := manticore.DefaultHTTPConfig("localhost:9308")
config.Timeout = 30 * time.Second
client = manticore.NewHTTPClient(*config)

// Создание схемы
err = client.CreateSchema()

// Индексирование документов
err = client.IndexDocuments(documents, vectors)

// Поиск через адаптер
aiConfig := models.DefaultAISearchConfig()
searchEngine := search.NewSearchEngine(client, vectorizer, aiConfig)
results, err := searchEngine.Search("query", models.SearchModeHybrid, 1, 10)
```

## Тестирование

Каждый модуль имеет соответствующие тесты:
```bash
go test ./internal/manticore/... -v
```

Тесты покрывают:
- Функциональность всех операций
- Обработку ошибок и edge cases
- Производительность и устойчивость
- Интеграцию между компонентами