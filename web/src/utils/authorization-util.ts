import { Authorization, Token, UserInfo } from '@/constants/authorization';
import { getSearchValue } from './common-util';

// Immediately handle ?auth from URL to avoid exposing token in address bar
(() => {
  if (typeof window === 'undefined') return;
  try {
    const url = new URL(window.location.href);
    const auth = url.searchParams.get('auth');
    if (auth) {
      // Store raw token without prefix
      localStorage.setItem(Authorization, auth);
      // Remove auth from URL without reloading
      url.searchParams.delete('auth');
      const newQuery = url.searchParams.toString();
      const newUrl = url.pathname + (newQuery ? `?${newQuery}` : '') + url.hash;
      window.history.replaceState(null, document.title, newUrl);
    }
  } catch {
    // noop: best-effort cleanup
  }
})();

const KeySet = [Authorization, Token, UserInfo];

const storage = {
  getAuthorization: () => {
    return localStorage.getItem(Authorization);
  },
  getToken: () => {
    return localStorage.getItem(Token);
  },
  getUserInfo: () => {
    return localStorage.getItem(UserInfo);
  },
  getUserInfoObject: () => {
    return JSON.parse(localStorage.getItem('userInfo') || '');
  },
  setAuthorization: (value: string) => {
    localStorage.setItem(Authorization, value);
  },
  setToken: (value: string) => {
    localStorage.setItem(Token, value);
  },
  setUserInfo: (value: string | Record<string, unknown>) => {
    let valueStr = typeof value !== 'string' ? JSON.stringify(value) : value;
    localStorage.setItem(UserInfo, valueStr);
  },
  setItems: (pairs: Record<string, string>) => {
    Object.entries(pairs).forEach(([key, value]) => {
      localStorage.setItem(key, value);
    });
  },
  removeAuthorization: () => {
    localStorage.removeItem(Authorization);
  },
  removeAll: () => {
    KeySet.forEach((x) => {
      localStorage.removeItem(x);
    });
  },
  setLanguage: (lng: string) => {
    localStorage.setItem('lng', lng);
  },
  getLanguage: (): string => {
    return localStorage.getItem('lng') as string;
  },
};

export const getAuthorization = () => {
  const auth = getSearchValue('auth');
  if (auth) {
    // Persist raw token from URL for subsequent requests
    storage.setAuthorization(auth);
    return 'Bearer ' + auth;
  }
  const stored = storage.getAuthorization() || '';
  if (!stored) return '';
  // Normalize: if already prefixed, return as-is; otherwise add prefix
  return stored.startsWith('Bearer ') ? stored : 'Bearer ' + stored;
};

export default storage;

// Will not jump to the login page
export function redirectToLogin() {
  window.location.href = location.origin + `/login`;
}
