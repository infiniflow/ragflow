import { useAuth } from '@/hooks/auth-hooks';
import { useEffect } from 'react';
import { useNavigate } from 'umi';
import './index.less';

const Login = () => {
  const navigate = useNavigate();
  const { isLogin } = useAuth();

  useEffect(() => {
    if (isLogin) {
      navigate('/');
    }
  }, [isLogin, navigate]);

  return (
    <div className="min-h-screen bg-white flex items-center justify-center">
      <div className="text-center">
        <div className="w-8 h-8 border-2 border-gray-300 border-t-blue-500 rounded-full animate-spin mx-auto mb-4"></div>
        <p className="text-gray-500 text-sm">加载中...</p>
      </div>
    </div>
  );
};

export default Login;
