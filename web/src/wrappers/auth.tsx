import { useAuth } from '@/hooks/auth-hooks';
import { Navigate, Outlet } from 'umi';

export default () => {
  const { isLogin } = useAuth();
  if (isLogin === true) {
    return <Outlet />;
  } else if (isLogin === false) {
    return <Navigate to="/login" />;
  }

  return <></>;
};
