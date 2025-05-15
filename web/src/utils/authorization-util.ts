import { Authorization, Token, UserInfo } from '@/constants/authorization';
import { Modal } from 'antd';
import { getSearchValue } from './common-util';
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
  const authorization = auth
    ? 'Bearer ' + auth
    : storage.getAuthorization() || '';

  return authorization;
};

export default storage;

function isURLSearchParamsEmpty(searchParams: URLSearchParams) {
  // Use entries() method to get iterator and try to get first element
  let firstItem = searchParams.entries().next();
  return firstItem.done; // If done is true, means there are no elements, i.e. searchParams is empty
}

const autoLogin = () => {
  // If current page is not login page, store current page URL in sessionStorage
  if (window.location.pathname !== '/login') {
    sessionStorage.setItem('auto_login_callback', window.location.href);
  }

  Modal.warning({
    title:
      'Auto login functionality needs to be implemented in web/src/utils/authorization-util.ts. This method is called when LoginType.AUTO is used.',
    onOk: () => {
      window.location.href = '/login';
    },
  });
  // Example oauth login:
  // window.location.href = `v1/user/login/github`;
};

export enum LoginType {
  AUTO = 'auto',
  NORMAL = 'normal',
}
// Will not jump to the login page
export function redirectToLogin({
  type,
  error,
}: { type?: LoginType; error?: string } = {}) {
  const loginType = type || LOGIN_TYPE || LoginType.NORMAL;
  const searchParams = new URLSearchParams();

  if (loginType === LoginType.AUTO && !error) {
    return autoLogin();
  }

  if (error) {
    searchParams.set('error', error);
  }

  if (isURLSearchParamsEmpty(searchParams)) {
    window.location.href = location.origin + '/login';
  } else {
    window.location.href =
      location.origin + `/login?${searchParams.toString()}`;
  }
}
