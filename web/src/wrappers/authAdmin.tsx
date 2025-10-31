import { Routes } from '@/routes';
import authorizationUtil from '@/utils/authorization-util';
import { Navigate, Outlet } from 'umi';

export default () => {
  const isLogin = !!authorizationUtil.getAuthorization();

  return isLogin ? <Outlet /> : <Navigate to={Routes.Admin} />;
};
