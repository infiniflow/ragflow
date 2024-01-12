import { connect } from 'umi';
import { Input, Form, Button, Checkbox } from 'antd';
import styles from './index.less';
import { rsaPsw } from '@/utils'
import { useState, useEffect } from 'react';

const View = ({
  dispatch,
}) => {
  const [title, setTitle] = useState('login')

  const changeTitle = () => {
    setTitle((title) => title === 'login' ? 'register' : 'login')
  }
  const [form] = Form.useForm();
  const [checkNick, setCheckNick] = useState(false);

  useEffect(() => {
    form.validateFields(['nickname']);
  }, [checkNick, form]);

  const onCheck = async () => {
    try {
      const params = await form.validateFields();

      var rsaPassWord = rsaPsw(params.password)
      if (title === 'login') {
        dispatch({
          type: 'loginModel/login',
          payload: {
            email: params.email,
            password: rsaPassWord
          }
        });
      } else {
        dispatch({
          type: 'loginModel/register',
          payload: {
            nickname: params.nickname,
            email: params.email,
            password: rsaPassWord
          }
        });
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };
  const formItemLayout = {
    labelCol: { span: 4 },
    wrapperCol: { span: 8 },
  };

  const formTailLayout = {
    labelCol: { span: 4 },
    wrapperCol: { span: 8, offset: 4 },
  };
  return (
    <div className={styles.loginContainer}>
      {title === 'login' ? '登录' : '注册'}
      <div className={styles.modal}>
        <Form form={form} name="dynamic_rule" style={{ maxWidth: 600 }}>
          <Form.Item
            {...formItemLayout}
            name="email"
            label="Email"
            rules={[{ required: true, message: 'Please input your name' }]}
          >
            <Input placeholder="Please input your name" />
          </Form.Item>
          {
            title === 'register' && <Form.Item
              {...formItemLayout}
              name="nickname"
              label="Nickname"
              rules={[{ required: checkNick, message: 'Please input your nickname' }]}
            >
              <Input placeholder="Please input your nickname" />
            </Form.Item>
          }
          <Form.Item
            {...formItemLayout}
            name="password"
            label="Password"
            rules={[{ required: true, message: 'Please input your name' }]}
          >
            <Input placeholder="Please input your name" />
          </Form.Item>
          <div>   {
            title === 'login' && (<div>
              没有账号?<a onClick={changeTitle}>注册</a>
            </div>)
          }
            {
              title === 'register' && (<div>
                已有账号?<a onClick={changeTitle}>登录</a>
              </div>)
            }</div>
          <div><a href="https://github.com/login/oauth/authorize?scope=user:email&client_id=302129228f0d96055bee">第三方登录</a></div>
          <Form.Item {...formTailLayout}>
            <Button type="primary" onClick={onCheck}>
              Check
            </Button>
          </Form.Item>
        </Form>
      </div>
    </div>
  );
};

export default connect(({ loginModel, loading }) => ({ loginModel, loading }))(View);
