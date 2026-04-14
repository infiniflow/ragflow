import { createContext, Dispatch, SetStateAction, useState } from 'react';
import { Outlet } from 'react-router';

import type { IUserInfo } from '@/interfaces/database/user-setting';
import authorizationUtil from '@/utils/authorization-util';

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

const getLocalStorageUserInfo = (): CurrentUserInfo => {
  const userInfo = authorizationUtil.getUserInfoObject();

  return userInfo
    ? {
        userInfo: userInfo,
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
