import { useContext } from 'react';
import { Navigate, Outlet } from 'react-router';

import { Routes } from '@/routes';
import { adminStorage } from '@/utils/authorization-util';
import { CurrentUserInfoContext } from './root-layout';

export default function AdminAuthorizedLayout() {
  const [{ userInfo }] = useContext(CurrentUserInfoContext);
  const isLoggedIn = !!adminStorage.getAuthorization() && userInfo;

  return isLoggedIn ? <Outlet /> : <Navigate to={Routes.Admin} />;
}
