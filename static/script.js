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
    lastSearchTime: 0
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
    
    pagination: document.getElementById('pagination'),
    prevButton: document.getElementById('prevButton'),
    nextButton: document.getElementById('nextButton'),
    paginationInfo: document.getElementById('paginationInfo'),
    
    reindexButton: document.getElementById('reindexButton'),
    
    initialState: document.getElementById('initialState'),
    emptyState: document.getElementById('emptyState'),
    errorState: document.getElementById('errorState'),
    errorMessage: document.getElementById('errorMessage'),
    retryButton: document.getElementById('retryButton')
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
        hybrid: 'Гибридный поиск'
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
    return (score * 100).toFixed(1) + '%';
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

function updateResultsInfo() {
    const totalText = state.totalResults === 1 ? 'результат' : 
                     state.totalResults < 5 ? 'результата' : 'результатов';
    
    elements.resultsCount.textContent = `${state.totalResults} ${totalText}`;
    elements.resultsMode.textContent = formatModeLabel(state.currentMode);
}

function updatePagination() {
    elements.prevButton.disabled = state.currentPage <= 1;
    elements.nextButton.disabled = state.currentPage >= state.totalPages;
    elements.paginationInfo.textContent = `Страница ${state.currentPage} из ${state.totalPages}`;
}

// ===== Results Rendering =====
function renderResults(results) {
    if (!results || results.length === 0) {
        showState('empty');
        return;
    }
    
    showState('results');
    
    elements.resultsList.innerHTML = results.map(result => {
        const scoreDisplay = result.score ? 
            `<div class="result-score">${formatScore(result.score)}</div>` : '';
        
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
    
    updateResultsInfo();
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
        
        // Render results
        renderResults(result.documents || []);
        
    } catch (error) {
        console.error('Search failed:', error);
        showError(`Ошибка поиска: ${error.message}`);
    } finally {
        setLoadingState(false);
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
        
    } catch (error) {
        console.error('Status check failed:', error);
        elements.connectionStatus.textContent = '⚠️ Ошибка';
        elements.connectionStatus.style.color = 'var(--warning-color)';
        elements.documentsCount.textContent = '-';
    }
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