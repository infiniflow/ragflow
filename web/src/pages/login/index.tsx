import { rsaPsw } from '@/utils';
import { Button, Checkbox, Form, Input } from 'antd';
import { useEffect, useState } from 'react';
import { Icon, useDispatch, useNavigate, useSelector } from 'umi';
import styles from './index.less';

const Login = () => {
  const [title, setTitle] = useState('login');
  let navigate = useNavigate();
  const dispatch = useDispatch();
  const effectsLoading: any = useSelector<any>( // TODO: Type needs to be improved
    (state) => state.loading.effects,
  );

  // TODO: When the server address request is not accessible, the value of dva-loading always remains true.

  const signLoading =
    effectsLoading['loginModel/login'] || effectsLoading['loginModel/register'];

  const changeTitle = () => {
    setTitle((title) => (title === 'login' ? 'register' : 'login'));
  };
  const [form] = Form.useForm();
  const [checkNick, setCheckNick] = useState(false);

  useEffect(() => {
    form.validateFields(['nickname']);
  }, [checkNick, form]);

  const onCheck = async () => {
    try {
      const params = await form.validateFields();

      var rsaPassWord = rsaPsw(params.password);
      if (title === 'login') {
        const retcode = await dispatch<any>({
          type: 'loginModel/login',
          payload: {
            email: params.email,
            password: rsaPassWord,
          },
        });
        if (retcode === 0) {
          navigate('/knowledge');
        }
      } else {
        // TODO: Type needs to be improved
        const retcode = await dispatch<any>({
          type: 'loginModel/register',
          payload: {
            nickname: params.nickname,
            email: params.email,
            password: rsaPassWord,
          },
        });
        if (retcode === 0) {
          setTitle('login');
        }
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };
  const formItemLayout = {
    labelCol: { span: 6 },
    // wrapperCol: { span: 8 },
  };

  const toGoogle = () => {
    window.location.href =
      'https://github.com/login/oauth/authorize?scope=user:email&client_id=302129228f0d96055bee';
  };
  return (
    <div className={styles.loginPage}>
      <div className={styles.loginLeft}>
        <div className={styles.modal}>
          <div className={styles.loginTitle}>
            <div>{title === 'login' ? 'Sign in' : 'Create an account'}</div>
            <span>
              {title === 'login'
                ? 'We’re so excited to see you again!'
                : 'Glad to have you on board!'}
            </span>
          </div>

          <Form
            form={form}
            layout="vertical"
            name="dynamic_rule"
            style={{ maxWidth: 600 }}
          >
            <Form.Item
              {...formItemLayout}
              name="email"
              label="Email"
              rules={[{ required: true, message: 'Please input value' }]}
            >
              <Input size="large" placeholder="Please input value" />
            </Form.Item>
            {title === 'register' && (
              <Form.Item
                {...formItemLayout}
                name="nickname"
                label="Nickname"
                rules={[
                  { required: true, message: 'Please input your nickname' },
                ]}
              >
                <Input size="large" placeholder="Please input your nickname" />
              </Form.Item>
            )}
            <Form.Item
              {...formItemLayout}
              name="password"
              label="Password"
              rules={[{ required: true, message: 'Please input value' }]}
            >
              <Input.Password size="large" placeholder="Please input value" />
            </Form.Item>
            {title === 'login' && (
              <Form.Item name="remember" valuePropName="checked">
                <Checkbox> Remember me</Checkbox>
              </Form.Item>
            )}
            <div>
              {' '}
              {title === 'login' && (
                <div>
                  Don’t have an account?
                  <Button type="link" onClick={changeTitle}>
                    Sign up
                  </Button>
                </div>
              )}
              {title === 'register' && (
                <div>
                  Already have an account?
                  <Button type="link" onClick={changeTitle}>
                    Sign in
                  </Button>
                </div>
              )}
            </div>
            <Button
              type="primary"
              block
              size="large"
              onClick={onCheck}
              loading={signLoading}
            >
              {title === 'login' ? 'Sign in' : 'Continue'}
            </Button>
            {title === 'login' && (
              <>
                {/* <Button
                  block
                  size="large"
                  onClick={toGoogle}
                  style={{ marginTop: 15 }}
                >
                  <div>
                    <Icon
                      icon="local:google"
                      style={{ verticalAlign: 'middle', marginRight: 5 }}
                    />
                    Sign in with Google
                  </div>
                </Button> */}
                <Button
                  block
                  size="large"
                  onClick={toGoogle}
                  style={{ marginTop: 15 }}
                >
                  <div>
                    <Icon
                      icon="local:github"
                      style={{ verticalAlign: 'middle', marginRight: 5 }}
                    />
                    Sign in with Github
                  </div>
                </Button>
              </>
            )}
          </Form>
        </div>
      </div>
      <div className={styles.loginRight}></div>
    </div>
  );
};

export default Login;
