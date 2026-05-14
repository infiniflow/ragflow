// LLM list cache utility

interface LlmCache {
  data: Record<string, any>;
  timestamp: number;
}

const CACHE_KEY = 'ragflow_llm_list_cache';
const CACHE_DURATION = 5 * 60 * 1000; // 5 minutes

// Get cached LLM list
export function getCachedLlmList(): Record<string, any> | null {
  try {
    const cached = localStorage.getItem(CACHE_KEY);
    if (!cached) return null;

    const parsed: LlmCache = JSON.parse(cached);
    const now = Date.now();

    // Check if cache is expired
    if (now - parsed.timestamp > CACHE_DURATION) {
      clearLlmCache();
      return null;
    }

    return parsed.data;
  } catch (error) {
    console.error('Error getting cached LLM list:', error);
    clearLlmCache();
    return null;
  }
}

// Set LLM list to cache
export function setCachedLlmList(data: Record<string, any>): void {
  try {
    const cache: LlmCache = {
      data,
      timestamp: Date.now(),
    };
    localStorage.setItem(CACHE_KEY, JSON.stringify(cache));
  } catch (error) {
    console.error('Error setting cached LLM list:', error);
  }
}

// Clear LLM list cache
export function clearLlmCache(): void {
  try {
    localStorage.removeItem(CACHE_KEY);
  } catch (error) {
    console.error('Error clearing LLM cache:', error);
  }
}

// Check if LLM list is cached and not expired
export function isLlmListCached(): boolean {
  return getCachedLlmList() !== null;
}
