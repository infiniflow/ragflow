import { useAuth } from '@/hooks/auth-hooks';
import { redirectToLogin } from '@/utils/authorization-util';
import { Outlet } from 'react-router';
import { Header } from './next-header';

export default function NextLayout() {
  const { isLogin } = useAuth();

  if (isLogin === false) {
    redirectToLogin();
    return null;
  }
  if (isLogin === null) return null;

  return (
    <main className="h-full flex flex-col">
      <Header />
      <Outlet />
    </main>
  );
}
