import { useLogin, useRegister } from '@/hooks/login-hooks';
import { rsaPsw } from '@/utils';
import { Button, Form, Input } from 'antd';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import styles from './index.less';

const Login = () => {
  const { loading: signLoading } = useLogin();
  const { register, loading: registerLoading } = useRegister();
  const { t } = useTranslation('translation', { keyPrefix: 'login' });
  const loading = signLoading || registerLoading;

  const [form] = Form.useForm();

  useEffect(() => {
    form.validateFields(['nickname']);
  }, [form]);

  const onCheck = async () => {
    try {
      const params = await form.validateFields();

      const rsaPassWord = rsaPsw(params.password) as string;

      const code = await register({
        nickname: params.nickname,
        email: params.email,
        password: rsaPassWord,
      });
      if (code !== 0) {
         console.log('register worry')
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
    <div className={styles.leftContainer}>
      <div className={styles.loginTitle}>
        <div>
          {t('register')}
        </div>
      </div>

      <Form
        form={form}
        layout="vertical"
        name="dynamic_rule"
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
          name="nickname"
          label={t('nicknameLabel')}
          rules={[{ required: true, message: t('nicknamePlaceholder') }]}
        >
          <Input size="large" placeholder={t('nicknamePlaceholder')} />
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
            defaultValue="1eey56MH01459@"
            onPressEnter={onCheck}
            disabled={true}
          />
        </Form.Item>
        <Button
          type="primary"
          block
          size="large"
          onClick={onCheck}
          loading={loading}
        >
          {t('register')}
        </Button>
      </Form>
    </div>
  );
};

export default Login;
