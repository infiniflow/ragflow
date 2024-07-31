import { Button, Result } from 'antd';
import { history } from 'umi';

const NoFoundPage = () => {
  return (
    <Result
      status="404"
      title="404"
      subTitle="Không tìm thấy trang, vui lòng nhập địa chỉ chính xác."
      extra={
        <Button type="primary" onClick={() => history.push('/')}>
          Trở về trang chủ
        </Button>
      }
    />
  );
};

export default NoFoundPage;
