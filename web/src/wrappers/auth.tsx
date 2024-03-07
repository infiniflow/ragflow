import { useAuth } from '@/hooks/authHook';
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
