import { createContext, Dispatch, SetStateAction, useState } from 'react';
import { Outlet } from 'react-router';

import type { IUserInfo } from '@/interfaces/database/user-setting';
import { adminStorage } from '@/utils/authorization-util';

type LocalStoragePersistedUserInfo = {
  avatar: unknown;
  name: string;
  email: string;
};

export type CurrentUserInfo =
  | {
      userInfo: null;
      source: null;
    }
  | {
      userInfo: AdminService.LoginData | IUserInfo;
      source: 'serverRequest';
    }
  | {
      userInfo: LocalStoragePersistedUserInfo;
      source: 'localStorage';
    };

const parseLocalStorageUserInfo = (
  value: Record<string, unknown> | null,
): LocalStoragePersistedUserInfo | null => {
  if (!value) {
    return null;
  }
  if (typeof value.name !== 'string' || typeof value.email !== 'string') {
    return null;
  }
  return {
    avatar: value.avatar,
    name: value.name,
    email: value.email,
  };
};

const getLocalStorageUserInfo = (): CurrentUserInfo => {
  const userInfo = parseLocalStorageUserInfo(adminStorage.getUserInfoObject());

  return userInfo
    ? {
        userInfo,
        source: 'localStorage',
      }
    : {
        userInfo: null,
        source: null,
      };
};

export const CurrentUserInfoContext = createContext<
  [CurrentUserInfo, Dispatch<SetStateAction<CurrentUserInfo>>]
>([getLocalStorageUserInfo(), () => {}]);

const AdminRootLayout = () => {
  const userInfoCtx = useState<CurrentUserInfo>(getLocalStorageUserInfo());

  return (
    <CurrentUserInfoContext.Provider value={userInfoCtx}>
      <Outlet context={userInfoCtx} />
    </CurrentUserInfoContext.Provider>
  );
};

export default AdminRootLayout;
