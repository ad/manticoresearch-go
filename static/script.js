// ===== Application State =====
const state = {
    currentQuery: '',
    currentMode: 'basic',
    currentPage: 1,
    currentLimit: 10,
    totalResults: 0,
    totalPages: 1,
    isSearching: false,
    searchDebounceTimer: null,
    lastSearchTime: 0,
    // AI Search state
    aiSearchEnabled: false,
    aiModel: null,
    aiSearchHealthy: false
};

// ===== DOM Elements =====
const elements = {
    searchForm: document.getElementById('searchForm'),
    searchInput: document.getElementById('searchInput'),
    searchLoader: document.getElementById('searchLoader'),
    searchModeSelect: document.getElementById('searchMode'),
    searchButton: document.querySelector('.search-button'),
    
    statusBar: document.getElementById('statusBar'),
    connectionStatus: document.getElementById('connectionStatus'),
    documentsCount: document.getElementById('documentsCount'),
    
    resultsSection: document.getElementById('resultsSection'),
    resultsInfo: document.getElementById('resultsInfo'),
    resultsCount: document.getElementById('resultsCount'),
    resultsMode: document.getElementById('resultsMode'),
    resultsList: document.getElementById('resultsList'),
    
    // AI Results Info elements
    aiResultsInfo: document.getElementById('aiResultsInfo'),
    aiModelBadge: document.getElementById('aiModelBadge'),
    aiModelInResults: document.getElementById('aiModelInResults'),
    aiFallbackNotice: document.getElementById('aiFallbackNotice'),
    fallbackText: document.getElementById('fallbackText'),
    
    pagination: document.getElementById('pagination'),
    prevButton: document.getElementById('prevButton'),
    nextButton: document.getElementById('nextButton'),
    paginationInfo: document.getElementById('paginationInfo'),
    
    reindexButton: document.getElementById('reindexButton'),
    
    initialState: document.getElementById('initialState'),
    emptyState: document.getElementById('emptyState'),
    errorState: document.getElementById('errorState'),
    errorMessage: document.getElementById('errorMessage'),
    retryButton: document.getElementById('retryButton'),
    
    // AI Search elements
    aiModelInfo: document.getElementById('aiModelInfo'),
    aiModelName: document.getElementById('aiModelName'),
    aiModelStatus: document.getElementById('aiModelStatus'),
    aiStatusIndicator: document.getElementById('aiStatusIndicator'),
    aiStatusText: document.getElementById('aiStatusText'),
    aiStatusItem: document.getElementById('aiStatusItem'),
    aiSearchStatus: document.getElementById('aiSearchStatus')
};

// ===== Configuration =====
const config = {
    DEBOUNCE_DELAY: 300, // ms
    MIN_QUERY_LENGTH: 1,
    MAX_RESULTS_PER_PAGE: 100,
    API_BASE_URL: '/api',
    SEARCH_MODES: {
        basic: 'Базовый поиск',
        fulltext: 'Полнотекстовый поиск', 
        vector: 'Векторный поиск',
        hybrid: 'Гибридный поиск',
        ai: 'AI Search (Semantic)'
    }
};

// ===== Utility Functions =====
function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

function formatScore(score) {
    if (score === undefined || score === null) return '';
    // Manticore returns absolute scores, not 0-1 range
    // Show score with 2 decimal places
    return score.toFixed(2);
}

function truncateText(text, maxLength = 200) {
    if (!text || text.length <= maxLength) return text;
    return text.substr(0, maxLength) + '...';
}

function formatModeLabel(mode) {
    return config.SEARCH_MODES[mode] || mode;
}

// ===== API Functions =====
async function makeAPIRequest(endpoint, options = {}) {
    try {
        const url = `${config.API_BASE_URL}${endpoint}`;
        const response = await fetch(url, {
            headers: {
                'Content-Type': 'application/json',
                ...options.headers
            },
            ...options
        });

        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }

        const data = await response.json();
        
        if (!data.success && data.error) {
            throw new Error(data.error);
        }
        
        return data.data;
    } catch (error) {
        console.error('API Request failed:', error);
        throw error;
    }
}

async function searchDocuments(query, mode = 'basic', page = 1, limit = 10) {
    const params = new URLSearchParams({
        query: query.trim(),
        mode,
        page: page.toString(),
        limit: limit.toString()
    });
    
    return makeAPIRequest(`/search?${params}`);
}

async function getStatus() {
    return makeAPIRequest('/status');
}

async function reindexDocuments() {
    return makeAPIRequest('/reindex', { method: 'POST' });
}

// ===== UI State Management =====
function setLoadingState(isLoading) {
    state.isSearching = isLoading;
    
    if (isLoading) {
        document.body.classList.add('loading');
        elements.searchButton.disabled = true;
    } else {
        document.body.classList.remove('loading');
        elements.searchButton.disabled = false;
    }
}

function showState(stateName) {
    // Hide all states
    elements.initialState.style.display = 'none';
    elements.emptyState.style.display = 'none';
    elements.errorState.style.display = 'none';
    elements.resultsInfo.style.display = 'none';
    elements.resultsList.innerHTML = '';
    elements.pagination.style.display = 'none';
    
    // Show requested state
    if (stateName === 'initial') {
        elements.initialState.style.display = 'block';
    } else if (stateName === 'empty') {
        elements.emptyState.style.display = 'block';
    } else if (stateName === 'error') {
        elements.errorState.style.display = 'block';
    } else if (stateName === 'results') {
        elements.resultsInfo.style.display = 'flex';
        if (state.totalPages > 1) {
            elements.pagination.style.display = 'flex';
        }
    }
}

function updateResultsInfo(searchResponse = null) {
    const totalText = state.totalResults === 1 ? 'результат' : 
                     state.totalResults < 5 ? 'результата' : 'результатов';
    
    elements.resultsCount.textContent = `${state.totalResults} ${totalText}`;
    
    // Enhanced mode display for AI search
    let modeText = formatModeLabel(state.currentMode);
    
    // Handle AI search results info
    if (state.currentMode === 'ai' && searchResponse) {
        // Show AI results info section
        elements.aiResultsInfo.style.display = 'flex';
        
        // Update AI model badge
        if (searchResponse.ai_model) {
            elements.aiModelInResults.textContent = searchResponse.ai_model;
            elements.aiModelBadge.title = `AI Model: ${searchResponse.ai_model}`;
        } else if (state.aiModel) {
            elements.aiModelInResults.textContent = state.aiModel;
            elements.aiModelBadge.title = `AI Model: ${state.aiModel}`;
        }
        
        // Handle different AI search modes and fallbacks
        if (searchResponse.mode) {
            if (searchResponse.mode.includes('fallback')) {
                elements.aiFallbackNotice.style.display = 'inline-flex';
                elements.fallbackText.textContent = 'AI search failed, using hybrid';
                elements.aiFallbackNotice.title = 'AI search encountered an error and fell back to hybrid search';
                modeText = 'Гибридный поиск (AI Fallback)';
            } else if (searchResponse.mode.includes('degraded')) {
                elements.aiFallbackNotice.style.display = 'inline-flex';
                elements.fallbackText.textContent = 'AI search degraded to hybrid';
                elements.aiFallbackNotice.title = 'AI search was not available and degraded to hybrid search';
                modeText = 'Гибридный поиск (AI Degraded)';
            } else {
                elements.aiFallbackNotice.style.display = 'none';
                modeText = 'AI Search (Semantic)';
            }
        } else {
            elements.aiFallbackNotice.style.display = 'none';
        }
    } else if (searchResponse && searchResponse.mode && searchResponse.mode.includes('AI')) {
        // Handle cases where AI search info should be shown even if current mode isn't 'ai'
        elements.aiResultsInfo.style.display = 'flex';
        
        if (state.aiModel) {
            elements.aiModelInResults.textContent = state.aiModel;
            elements.aiModelBadge.title = `AI Model: ${state.aiModel}`;
        }
        
        if (searchResponse.mode.includes('fallback')) {
            elements.aiFallbackNotice.style.display = 'inline-flex';
            elements.fallbackText.textContent = 'AI search failed';
            elements.aiFallbackNotice.title = 'AI search failed and fell back to hybrid search';
            modeText = 'Гибридный поиск (AI Fallback)';
        } else if (searchResponse.mode.includes('degraded')) {
            elements.aiFallbackNotice.style.display = 'inline-flex';
            elements.fallbackText.textContent = 'AI search degraded';
            elements.aiFallbackNotice.title = 'AI search was degraded to hybrid search';
            modeText = 'Гибридный поиск (AI Degraded)';
        }
    } else {
        // Hide AI results info for non-AI searches
        elements.aiResultsInfo.style.display = 'none';
    }
    
    elements.resultsMode.textContent = modeText;
}

function updatePagination() {
    // Update pagination button states
    elements.prevButton.disabled = state.currentPage <= 1;
    elements.nextButton.disabled = state.currentPage >= state.totalPages;
    
    // Update pagination info text
    elements.paginationInfo.textContent = `Страница ${state.currentPage} из ${state.totalPages}`;
    
    // Show/hide pagination based on total pages
    if (state.totalPages > 1) {
        elements.pagination.style.display = 'flex';
    } else {
        elements.pagination.style.display = 'none';
    }
}

function updatePagination() {
    elements.prevButton.disabled = state.currentPage <= 1;
    elements.nextButton.disabled = state.currentPage >= state.totalPages;
    elements.paginationInfo.textContent = `Страница ${state.currentPage} из ${state.totalPages}`;
}

function updateAISearchUI() {
    const isAIMode = state.currentMode === 'ai';
    
    if (isAIMode && state.aiSearchEnabled) {
        elements.aiModelInfo.style.display = 'block';
        elements.aiModelName.textContent = state.aiModel || 'Unknown';
        
        if (state.aiSearchHealthy) {
            elements.aiStatusIndicator.textContent = '🟢';
            elements.aiStatusText.textContent = 'Ready';
            elements.aiStatusText.style.color = 'var(--success-color)';
        } else {
            elements.aiStatusIndicator.textContent = '🔴';
            elements.aiStatusText.textContent = 'Unavailable';
            elements.aiStatusText.style.color = 'var(--error-color)';
        }
    } else {
        elements.aiModelInfo.style.display = 'none';
    }
    
    // Disable AI search option if not available
    const aiOption = elements.searchModeSelect.querySelector('option[value="ai"]');
    if (aiOption) {
        aiOption.disabled = !state.aiSearchEnabled;
        if (!state.aiSearchEnabled) {
            aiOption.textContent = 'AI Search (Недоступен)';
        } else {
            aiOption.textContent = 'AI Search (Semantic)';
        }
    }
}

// ===== Results Rendering =====
function renderResults(results, searchResponse = null) {
    if (!results || results.length === 0) {
        showState('empty');
        return;
    }
    
    showState('results');
    
    // Enhanced score handling for different search modes
    const maxScore = Math.max(...results.map(r => r.score || 0));
    const isAISearch = state.currentMode === 'ai';
    
    elements.resultsList.innerHTML = results.map(result => {
        let scoreDisplay = '';
        
        if (result.score !== undefined && result.score !== null) {
            if (isAISearch) {
                // For AI search, show semantic similarity score
                // AI search typically returns scores between 0-1 or cosine similarity
                const semanticScore = result.score > 1 ? 
                    (result.score / 100).toFixed(3) : result.score.toFixed(3);
                scoreDisplay = `<div class="result-score ai-score" title="Semantic Similarity Score">${semanticScore}</div>`;
            } else {
                // For other search modes, show normalized percentage
                const normalizedScore = maxScore > 0 ? 
                    ((result.score / maxScore) * 100).toFixed(1) + '%' : result.score.toFixed(2);
                scoreDisplay = `<div class="result-score">${normalizedScore}</div>`;
            }
        }
        
        const contentDisplay = result.document && result.document.content ? 
            `<div class="result-content">${truncateText(result.document.content)}</div>` : '';
            
        return `
            <div class="result-item" onclick="openResult('${result.document?.url}')">
                <div class="result-header">
                    <h3 class="result-title">${escapeHtml(result.document?.title || 'Без названия')}</h3>
                    ${scoreDisplay}
                </div>
                <a href="${result.document?.url}" class="result-url" onclick="event.stopPropagation()">${result.document?.url || ''}</a>
                ${contentDisplay}
            </div>
        `;
    }).join('');
    
    updateResultsInfo(searchResponse);
    updatePagination();
}

function escapeHtml(unsafe) {
    return unsafe
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

function openResult(url) {
    if (url && url !== 'undefined') {
        window.open(url, '_blank');
    }
}

// ===== Search Functions =====
async function performSearch(query = state.currentQuery, mode = state.currentMode, page = state.currentPage) {
    if (!query || query.trim().length < config.MIN_QUERY_LENGTH) {
        showState('initial');
        return;
    }
    
    // Prevent duplicate searches
    const searchSignature = `${query}-${mode}-${page}`;
    const now = Date.now();
    if (state.lastSearchSignature === searchSignature && (now - state.lastSearchTime) < 1000) {
        return;
    }
    
    setLoadingState(true);
    
    try {
        state.lastSearchTime = now;
        state.lastSearchSignature = searchSignature;
        
        const result = await searchDocuments(query, mode, page, state.currentLimit);
        
        // Update state
        state.currentQuery = query;
        state.currentMode = mode;
        state.currentPage = page;
        state.totalResults = result.total || 0;
        state.totalPages = Math.max(1, Math.ceil(state.totalResults / state.currentLimit));
        
        // Render results with search response metadata
        renderResults(result.documents || [], result);
        
    } catch (error) {
        console.error('Search failed:', error);
        handleSearchError(error, mode, query);
    } finally {
        setLoadingState(false);
    }
}

function handleSearchError(error, mode, query) {
    // Check if this is an AI search specific error
    if (mode === 'ai') {
        handleAISearchError(error, query);
    } else {
        // Standard error handling for other search modes
        showError(`Ошибка поиска: ${error.message}`);
    }
}

function handleAISearchError(error, query) {
    console.error('AI Search error:', error);
    
    // Try to parse error response for additional context
    let errorData = null;
    try {
        // If error.message contains JSON, try to parse it
        if (error.message.includes('{')) {
            const jsonStart = error.message.indexOf('{');
            const jsonStr = error.message.substring(jsonStart);
            errorData = JSON.parse(jsonStr);
        }
    } catch (parseError) {
        // Ignore parsing errors, use original error
    }
    
    if (errorData && errorData.error_type) {
        switch (errorData.error_type) {
            case 'ai_search_unavailable':
                showAISearchUnavailableError(errorData, query);
                break;
            case 'ai_search_failure':
                showAISearchFailureError(errorData, query);
                break;
            default:
                showGenericAISearchError(error.message, query);
        }
    } else {
        showGenericAISearchError(error.message, query);
    }
}

function showAISearchUnavailableError(errorData, query) {
    const suggestedModes = errorData.suggested_modes || ['hybrid', 'fulltext'];
    const reason = errorData.reason || 'Service unavailable';
    
    const errorMessage = `
        <div class="ai-error-container">
            <div class="ai-error-title">🤖 AI Search недоступен</div>
            <div class="ai-error-reason">Причина: ${reason}</div>
            <div class="ai-error-suggestion">
                Попробуйте один из альтернативных режимов поиска:
                <div class="suggested-modes">
                    ${suggestedModes.map(mode => 
                        `<button class="mode-suggestion-btn" onclick="switchToMode('${mode}', '${query}')">${formatModeLabel(mode)}</button>`
                    ).join('')}
                </div>
            </div>
        </div>
    `;
    
    elements.errorMessage.innerHTML = errorMessage;
    showState('error');
}

function showAISearchFailureError(errorData, query) {
    const aiError = errorData.ai_error || 'Unknown AI error';
    const fallbackError = errorData.fallback_error || 'Unknown fallback error';
    const errorCategory = errorData.error_category || 'unknown';
    const retrySuggested = errorData.retry_suggested || false;
    const suggestedModes = errorData.suggested_modes || ['hybrid', 'fulltext'];
    
    // Customize error message based on category
    let categoryMessage = '';
    let categoryIcon = '🤖';
    
    switch (errorCategory) {
        case 'timeout':
            categoryMessage = 'Превышено время ожидания AI поиска';
            categoryIcon = '⏱️';
            break;
        case 'network':
            categoryMessage = 'Проблема с сетевым соединением';
            categoryIcon = '🌐';
            break;
        case 'embedding':
            categoryMessage = 'Ошибка генерации векторных представлений';
            categoryIcon = '🧠';
            break;
        case 'model':
            categoryMessage = 'Проблема с AI моделью';
            categoryIcon = '🤖';
            break;
        case 'server_error':
            categoryMessage = 'Ошибка сервера AI поиска';
            categoryIcon = '🔧';
            break;
        default:
            categoryMessage = 'AI Search не удался';
            categoryIcon = '🤖';
    }
    
    const retryButton = retrySuggested ? 
        `<button class="mode-suggestion-btn retry-btn" onclick="retryAISearch('${query}')">Попробовать снова</button>` : '';
    
    const errorMessage = `
        <div class="ai-error-container">
            <div class="ai-error-title">${categoryIcon} ${categoryMessage}</div>
            <div class="ai-error-details">
                <div class="error-detail">
                    <strong>AI Search:</strong> ${aiError}
                </div>
                <div class="error-detail">
                    <strong>Резервный поиск:</strong> ${fallbackError}
                </div>
            </div>
            <div class="ai-error-suggestion">
                ${retrySuggested ? 'Попробуйте повторить запрос или выберите другой режим:' : 'Попробуйте другой режим поиска:'}
                <div class="suggested-modes">
                    ${retryButton}
                    ${suggestedModes.map(mode => 
                        `<button class="mode-suggestion-btn" onclick="switchToMode('${mode}', '${query}')">${formatModeLabel(mode)}</button>`
                    ).join('')}
                </div>
            </div>
        </div>
    `;
    
    elements.errorMessage.innerHTML = errorMessage;
    showState('error');
}

function retryAISearch(query) {
    // Retry the AI search with the same query
    if (query) {
        elements.searchInput.value = query;
        elements.searchModeSelect.value = 'ai';
        state.currentMode = 'ai';
        updateAISearchUI();
        performSearch(query, 'ai', 1);
    }
}

function showGenericAISearchError(errorMessage, query) {
    const message = `
        <div class="ai-error-container">
            <div class="ai-error-title">🤖 Ошибка AI Search</div>
            <div class="ai-error-details">${errorMessage}</div>
            <div class="ai-error-suggestion">
                Попробуйте другой режим поиска:
                <div class="suggested-modes">
                    <button class="mode-suggestion-btn" onclick="switchToMode('hybrid', '${query}')">Гибридный поиск</button>
                    <button class="mode-suggestion-btn" onclick="switchToMode('fulltext', '${query}')">Полнотекстовый поиск</button>
                </div>
            </div>
        </div>
    `;
    
    elements.errorMessage.innerHTML = message;
    showState('error');
}

function switchToMode(newMode, query) {
    // Update the search mode select
    elements.searchModeSelect.value = newMode;
    state.currentMode = newMode;
    
    // Update AI search UI
    updateAISearchUI();
    
    // Perform search with new mode
    if (query) {
        elements.searchInput.value = query;
        performSearch(query, newMode, 1);
    }
}

function showError(message) {
    elements.errorMessage.textContent = message;
    showState('error');
}

// Debounced search function
const debouncedSearch = debounce((query, mode) => {
    performSearch(query, mode, 1); // Reset to first page on new search
}, config.DEBOUNCE_DELAY);

// ===== Status Functions =====
async function updateStatus() {
    try {
        const status = await getStatus();
        
        if (status.manticore_healthy) {
            elements.connectionStatus.textContent = '✅ Подключено';
            elements.connectionStatus.style.color = 'var(--success-color)';
        } else {
            elements.connectionStatus.textContent = '❌ Отключено';
            elements.connectionStatus.style.color = 'var(--error-color)';
        }
        
        elements.documentsCount.textContent = status.documents_loaded || 0;
        
        // Update AI search status
        updateAISearchStatus(status);
        
    } catch (error) {
        console.error('Status check failed:', error);
        elements.connectionStatus.textContent = '⚠️ Ошибка';
        elements.connectionStatus.style.color = 'var(--warning-color)';
        elements.documentsCount.textContent = '-';
        
        // Reset AI search status on error
        state.aiSearchEnabled = false;
        state.aiSearchHealthy = false;
        updateAISearchUI();
    }
}

function updateAISearchStatus(status) {
    // Update AI search state
    state.aiSearchEnabled = status.ai_search_enabled || false;
    state.aiModel = status.ai_model || null;
    state.aiSearchHealthy = status.ai_search_healthy || false;
    
    // Update AI search status in status bar
    if (state.aiSearchEnabled) {
        elements.aiStatusItem.style.display = 'block';
        if (state.aiSearchHealthy) {
            elements.aiSearchStatus.textContent = '✅ Готов';
            elements.aiSearchStatus.style.color = 'var(--success-color)';
        } else {
            elements.aiSearchStatus.textContent = '⚠️ Недоступен';
            elements.aiSearchStatus.style.color = 'var(--warning-color)';
        }
    } else {
        elements.aiStatusItem.style.display = 'none';
    }
    
    // Update AI model info
    updateAISearchUI();
}

// ===== Reindex Function =====
async function handleReindex() {
    const button = elements.reindexButton;
    const originalText = button.innerHTML;
    
    try {
        button.disabled = true;
        button.innerHTML = '<span class="reindex-icon">🔄</span> Переиндексация...';
        
        await reindexDocuments();
        
        // Update status after reindex
        await updateStatus();
        
        // Show success feedback
        button.innerHTML = '<span class="reindex-icon">✅</span> Готово';
        setTimeout(() => {
            button.innerHTML = originalText;
            button.disabled = false;
        }, 2000);
        
        // Refresh current search if exists
        if (state.currentQuery) {
            performSearch();
        }
        
    } catch (error) {
        console.error('Reindex failed:', error);
        button.innerHTML = '<span class="reindex-icon">❌</span> Ошибка';
        setTimeout(() => {
            button.innerHTML = originalText;
            button.disabled = false;
        }, 2000);
        
        showError(`Ошибка переиндексации: ${error.message}`);
    }
}

// ===== Event Handlers =====
function setupEventListeners() {
    // Search form submission
    elements.searchForm.addEventListener('submit', (e) => {
        e.preventDefault();
        const query = elements.searchInput.value.trim();
        const mode = elements.searchModeSelect.value;
        if (query) {
            performSearch(query, mode, 1);
        }
    });
    
    // Real-time search on input
    elements.searchInput.addEventListener('input', (e) => {
        const query = e.target.value.trim();
        const mode = elements.searchModeSelect.value;
        
        if (query.length >= config.MIN_QUERY_LENGTH) {
            debouncedSearch(query, mode);
        } else {
            showState('initial');
        }
    });
    
    // Search mode change
    elements.searchModeSelect.addEventListener('change', (e) => {
        const query = elements.searchInput.value.trim();
        const mode = e.target.value;
        
        // Update current mode and UI
        state.currentMode = mode;
        updateAISearchUI();
        
        if (query.length >= config.MIN_QUERY_LENGTH) {
            debouncedSearch(query, mode);
        }
    });
    
    // Pagination
    elements.prevButton.addEventListener('click', () => {
        if (state.currentPage > 1) {
            performSearch(state.currentQuery, state.currentMode, state.currentPage - 1);
        }
    });
    
    elements.nextButton.addEventListener('click', () => {
        if (state.currentPage < state.totalPages) {
            performSearch(state.currentQuery, state.currentMode, state.currentPage + 1);
        }
    });
    
    // Reindex button
    elements.reindexButton.addEventListener('click', handleReindex);
    
    // Retry button
    elements.retryButton.addEventListener('click', () => {
        if (state.currentQuery) {
            performSearch();
        } else {
            showState('initial');
        }
    });
    
    // Keyboard shortcuts
    document.addEventListener('keydown', (e) => {
        // Focus search input with Ctrl/Cmd + K
        if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
            e.preventDefault();
            elements.searchInput.focus();
        }
        
        // Clear search with Escape
        if (e.key === 'Escape' && document.activeElement === elements.searchInput) {
            elements.searchInput.value = '';
            elements.searchInput.blur();
            showState('initial');
        }
    });
    
    // Focus search input on page load
    elements.searchInput.focus();
}

// ===== Initialization =====
async function initializeApp() {
    console.log('🚀 Initializing Manticore Search Tester...');
    
    try {
        // Setup event listeners
        setupEventListeners();
        
        // Update initial status
        await updateStatus();
        
        // Initialize AI search UI
        updateAISearchUI();
        
        // Show initial state
        showState('initial');
        
        console.log('✅ App initialized successfully');
        
    } catch (error) {
        console.error('❌ Failed to initialize app:', error);
        showError(`Не удалось инициализировать приложение: ${error.message}`);
    }
}

// ===== Auto-refresh Status =====
function startStatusUpdates() {
    // Update status every 30 seconds
    setInterval(updateStatus, 30000);
}

// ===== App Entry Point =====
document.addEventListener('DOMContentLoaded', () => {
    initializeApp();
    startStatusUpdates();
});

// ===== Global Error Handler =====
window.addEventListener('error', (e) => {
    console.error('Global error:', e.error);
    if (!state.isSearching) {
        showError('Произошла неожиданная ошибка. Попробуйте перезагрузить страницу.');
    }
});

// ===== Service Worker Registration (Optional - for future PWA features) =====
if ('serviceWorker' in navigator) {
    window.addEventListener('load', () => {
        // Service worker can be added in future for offline functionality
        console.log('Service Worker support detected');
    });
}