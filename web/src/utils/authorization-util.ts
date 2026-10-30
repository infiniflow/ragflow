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

type CredentialKeys = {
  authorization: string;
  token: string;
  userInfo: string;
};

const UserCredentialKeys: CredentialKeys = {
  authorization: Authorization,
  token: Token,
  userInfo: UserInfo,
};

const AdminCredentialKeys: CredentialKeys = {
  authorization: AdminAuthorization,
  token: AdminToken,
  userInfo: AdminUserInfo,
};

type LoginCredentials = {
  authorization: string;
  token: string;
  userInfo: string;
};

const createCredentialStorage = (keys: CredentialKeys) => ({
  getAuthorization: (): string =>
    localStorage.getItem(keys.authorization) ?? '',
  getUserInfoObject: () => {
    const userInfoStr = localStorage.getItem(keys.userInfo);
    return userInfoStr ? JSON.parse(userInfoStr) : null;
  },
  saveLoginCredentials: (credentials: LoginCredentials) => {
    localStorage.setItem(keys.authorization, credentials.authorization);
    localStorage.setItem(keys.token, credentials.token);
    localStorage.setItem(keys.userInfo, credentials.userInfo);
  },
  removeAll: () => {
    Object.values(keys).forEach((key) => {
      localStorage.removeItem(key);
    });
  },
});

export type CredentialStorage = ReturnType<typeof createCredentialStorage>;

const storage = {
  ...createCredentialStorage(UserCredentialKeys),
  setLanguage: (lng: string) => {
    localStorage.setItem('lng', lng);
  },
  getLanguage: (): string => {
    return localStorage.getItem('lng') as string;
  },
  setThinkingLevel: (level: string) => {
    localStorage.setItem(ThinkingLevel, level);
  },
  // Only Naive (0) and Agentic (5) are offered; a level persisted by a
  // since-removed option falls back to the Naive default rather than reaching
  // the request as NaN.
  getThinkingLevel: (): string => {
    const level = localStorage.getItem(ThinkingLevel);
    return level === '0' || level === '5' ? level : '0';
  },
};

export const adminStorage = createCredentialStorage(AdminCredentialKeys);

export const getAuthorization = () => {
  const auth = getSearchValue('auth');
  const authorization = auth ? 'Bearer ' + auth : storage.getAuthorization();

  return authorization;
};

export default storage;

// Will not jump to the login page
export function redirectToLogin() {
  // const env = import.meta.env;
  window.location.href = location.origin + `/login`;
}
