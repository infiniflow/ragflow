import { Button, Result } from 'antd';
import { history } from 'umi';

const NoFoundPage = () => {
  return (
    <Result
      status="404"
      title="404"
      subTitle="Page not found, please enter a correct address."
      extra={
        <Button type="primary" onClick={() => history.push('/')}>
          Business
        </Button>
      }
    />
  );
};

export default NoFoundPage;
