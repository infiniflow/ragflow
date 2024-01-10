import { connect } from 'umi';
import { Input, Form, Button, Checkbox } from 'antd';
// import md5 from 'md5';
import styles from './index.less';

const View = ({
  loginModel,
  dispatch,
  location,
}) => {
  const onFinish = (params: any) => {
    console.log('Success:', params);
    params.mail = params.username.replace(/^\s+|\s+$/g, '');
    params.passwordMd5 = params.password.replace(/^\s+|\s+$/g, '');

    dispatch({
      type: 'loginModel/login',
      payload: {
        mail: params.mail,
        authority: 1,
        passwordMd5: params.passwordMd5
        // passwordMd5: md5(params.passwordMd5).toLocaleUpperCase()
      }
    });
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };


  type FieldType = {
    username?: string;
    password?: string;
    remember?: string;
  };

  const onJump = () => {
    window.location.href = 'http://www.martechlab.cn/';
  };

  return (
    <div className={styles.loginContainer}>
      <div className={styles.modal}>
        <Form
          name="basic"
          labelCol={{ span: 8 }}
          wrapperCol={{ span: 16 }}
          style={{ maxWidth: 600 }}
          initialValues={{ remember: true }}
          onFinish={onFinish}
          onFinishFailed={onFinishFailed}
          autoComplete="off"
        >
          <Form.Item<FieldType>
            label="mail"
            name="username"
            rules={[{ required: true, message: 'Please input your mail!' }]}
          >
            <Input />
          </Form.Item>

          <Form.Item<FieldType>
            label="Password"
            name="password"
            rules={[{ required: true, message: 'Please input your password!' }]}
          >
            <Input.Password />
          </Form.Item>

          {/* <Form.Item<FieldType>
            name="remember"
            valuePropName="checked"
            wrapperCol={{ offset: 8, span: 16 }}
          >
            <Checkbox>Remember me</Checkbox>
          </Form.Item> */}

          <Form.Item wrapperCol={{ offset: 8, span: 16 }}>
            <Button type="primary" htmlType="submit" >
              Submit
            </Button>
          </Form.Item>
        </Form>
      </div>
    </div>
  );
};

export default connect(({ loginModel, loading }) => ({ loginModel, loading }))(View);
