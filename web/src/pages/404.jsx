import { Routes } from '@/routes';
import { Button, Result } from 'antd';
import { history, useLocation } from 'umi';

const NoFoundPage = () => {
  const location = useLocation();

  return (
    <Result
      status="404"
      title="404"
      subTitle="Page not found, please enter a correct address."
      extra={
        <Button
          type="primary"
          onClick={() => {
            history.push(
              location.pathname.startsWith(Routes.Admin) ? Routes.Admin : '/',
            );
          }}
        >
          Business
        </Button>
      }
    />
  );
};

export default NoFoundPage;
