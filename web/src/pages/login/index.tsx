import { useLogin } from '@/hooks/login-hooks';
import { rsaPsw } from '@/utils';
import { Button, Checkbox, Form, Input } from 'antd';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';
import RightPanel from './right-panel';
import SvgIcon from '@/components/svg-icon';

import styles from './index.less';

const Login = () => {
  const navigate = useNavigate();
  const { login, loading: signLoading } = useLogin();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const loading = signLoading;

  const [form] = Form.useForm();

  useEffect(() => {}, [form]);

  const onCheck = async () => {
    try {
      const params = await form.validateFields();

      const rsaPassWord = rsaPsw(params.password) as string;

      const code = await login({
        email: `${params.email}`.trim(),
        password: rsaPassWord,
      });
      if (code === 0) {
        navigate('/knowledge');
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };
  const formItemLayout = {
    labelCol: { span: 6 },
    // wrapperCol: { span: 8 },
  };

  return (
    <div className={styles.loginPage}>
      <div className={styles.loginLeft}>
        <RightPanel></RightPanel>
      </div>

      <div className={styles.loginRight}>
        <div className={styles.loginMark}>
          <SvgIcon name="login-mark" width={240} ></SvgIcon>
        </div>
        <div className={styles.leftContainer}>
          <div className={styles.loginTitle}>
            <div>
              {t('login')}
            </div>
            <span>
              {t('loginDescription')}
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
              label={t('emailLabel')}
              rules={[{ required: true, message: t('emailPlaceholder') }]}
            >
              <Input size="large" placeholder={t('emailPlaceholder')} />
            </Form.Item>
            <Form.Item
              {...formItemLayout}
              name="password"
              label={t('passwordLabel')}
              rules={[{ required: true, message: t('passwordPlaceholder') }]}
            >
              <Input.Password
                size="large"
                placeholder={t('passwordPlaceholder')}
                onPressEnter={onCheck}
              />
            </Form.Item>
            <Form.Item name="remember" valuePropName="checked">
              <Checkbox> {t('rememberMe')}</Checkbox>
            </Form.Item>
            <Button
              type="primary"
              block
              size="large"
              onClick={onCheck}
              loading={loading}
            >
              {t('login')}
            </Button>
          </Form>
        </div>
      </div>
    </div>
  );
};

export default Login;
