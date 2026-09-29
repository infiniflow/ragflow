/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import {
  AdminAuthorization,
  AdminToken,
  AdminUserInfo,
  Authorization,
  ThinkingLevel,
  Token,
  UserInfo,
} from '@/constants/authorization';
import { getSearchValue } from './common-util';

type SessionStorage = {
  getAuthorization: () => string | null;
  getToken: () => string | null;
  getUserInfo: () => string | null;
  getUserInfoObject: () => Record<string, unknown> | null;
  setAuthorization: (value: string) => void;
  setToken: (value: string) => void;
  setUserInfo: (value: string | Record<string, unknown>) => void;
  setItems: (pairs: Record<string, string>) => void;
  removeAuthorization: () => void;
  removeAll: () => void;
};

function createSessionStorage(
  authKey: string,
  tokenKey: string,
  userInfoKey: string,
): SessionStorage {
  const keySet = [authKey, tokenKey, userInfoKey];

  const logicalToPhysicalKey: Record<string, string> = {
    [Authorization]: authKey,
    [Token]: tokenKey,
    [UserInfo]: userInfoKey,
    [AdminAuthorization]: authKey,
    [AdminToken]: tokenKey,
    [AdminUserInfo]: userInfoKey,
  };

  return {
    getAuthorization: () => localStorage.getItem(authKey),
    getToken: () => localStorage.getItem(tokenKey),
    getUserInfo: () => localStorage.getItem(userInfoKey),
    getUserInfoObject: () => {
      const userInfoStr = localStorage.getItem(userInfoKey);
      return userInfoStr ? JSON.parse(userInfoStr) : null;
    },
    setAuthorization: (value: string) => {
      localStorage.setItem(authKey, value);
    },
    setToken: (value: string) => {
      localStorage.setItem(tokenKey, value);
    },
    setUserInfo: (value: string | Record<string, unknown>) => {
      const valueStr =
        typeof value !== 'string' ? JSON.stringify(value) : value;
      localStorage.setItem(userInfoKey, valueStr);
    },
    setItems: (pairs: Record<string, string>) => {
      Object.entries(pairs).forEach(([key, value]) => {
        const storageKey = logicalToPhysicalKey[key] ?? key;
        localStorage.setItem(storageKey, value);
      });
    },
    removeAuthorization: () => {
      localStorage.removeItem(authKey);
    },
    removeAll: () => {
      keySet.forEach((key) => {
        localStorage.removeItem(key);
      });
    },
  };
}

const userStorage = createSessionStorage(Authorization, Token, UserInfo);

export const adminStorage = createSessionStorage(
  AdminAuthorization,
  AdminToken,
  AdminUserInfo,
);

const languageStorage = {
  setLanguage: (lng: string) => {
    localStorage.setItem('lng', lng);
  },
  getLanguage: (): string => {
    return localStorage.getItem('lng') as string;
  },
  setThinkingLevel: (level: string) => {
    localStorage.setItem(ThinkingLevel, level);
  },
  getThinkingLevel: (): string => {
    return localStorage.getItem(ThinkingLevel) || '1';
  },
};

const storage = {
  ...userStorage,
  ...languageStorage,
};

export const getAuthorization = () => {
  const auth = getSearchValue('auth');
  const authorization = auth
    ? 'Bearer ' + auth
    : userStorage.getAuthorization() || '';

  return authorization;
};

export const getAdminAuthorization = () => {
  return adminStorage.getAuthorization() || '';
};

export default storage;

// Will not jump to the login page
export function redirectToLogin() {
  window.location.href = location.origin + `/login`;
}
