import { useOneNamespaceEffectsLoading } from '@/hooks/storeHooks';
import { useSaveSetting } from '@/hooks/userSettingHook';
import { rsaPsw } from '@/utils';
import { Button, Divider, Form, Input, Space } from 'antd';
import SettingTitle from '../components/setting-title';
import { useValidateSubmittable } from '../hooks';

import parentStyles from '../index.less';
import styles from './index.less';

type FieldType = {
  password?: string;
  new_password?: string;
  confirm_password?: string;
};

const tailLayout = {
  wrapperCol: { offset: 20, span: 4 },
};

const UserSettingPassword = () => {
  const loading = useOneNamespaceEffectsLoading('settingModel', ['setting']);
  const { form, submittable } = useValidateSubmittable();
  const saveSetting = useSaveSetting();

  const onFinish = (values: any) => {
    const password = rsaPsw(values.password) as string;
    const new_password = rsaPsw(values.new_password) as string;

    saveSetting({ password, new_password });
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  return (
    <section className={styles.passwordWrapper}>
      <SettingTitle
        title="Password"
        description="Please enter your current password to change your password."
      ></SettingTitle>
      <Divider />
      <Form
        colon={false}
        name="basic"
        labelAlign={'left'}
        labelCol={{ span: 8 }}
        wrapperCol={{ span: 16 }}
        style={{ width: '100%' }}
        initialValues={{ remember: true }}
        onFinish={onFinish}
        onFinishFailed={onFinishFailed}
        form={form}
        autoComplete="off"
        // requiredMark={'optional'}
      >
        <Form.Item<FieldType>
          label="Current password"
          name="password"
          rules={[
            {
              required: true,
              message: 'Please input your password!',
              whitespace: true,
            },
          ]}
        >
          <Input.Password />
        </Form.Item>
        <Divider />
        <Form.Item label="New password" required>
          <Form.Item<FieldType>
            noStyle
            name="new_password"
            rules={[
              {
                required: true,
                message: 'Please input your password!',
                whitespace: true,
              },
            ]}
          >
            <Input.Password />
          </Form.Item>
          <p className={parentStyles.itemDescription}>
            Your new password must be more than 8 characters.
          </p>
        </Form.Item>
        <Divider />
        <Form.Item<FieldType>
          label="Confirm new password"
          name="confirm_password"
          dependencies={['new_password']}
          rules={[
            {
              required: true,
              message: 'Please confirm your password!',
              whitespace: true,
            },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (!value || getFieldValue('new_password') === value) {
                  return Promise.resolve();
                }
                return Promise.reject(
                  new Error('The new password that you entered do not match!'),
                );
              },
            }),
          ]}
        >
          <Input.Password />
        </Form.Item>
        <Divider />
        <Form.Item
          {...tailLayout}
          shouldUpdate={(prevValues, curValues) =>
            prevValues.additional !== curValues.additional
          }
        >
          <Space>
            <Button htmlType="button">Cancel</Button>
            <Button
              type="primary"
              htmlType="submit"
              disabled={!submittable}
              loading={loading}
            >
              Save
            </Button>
          </Space>
        </Form.Item>
      </Form>
    </section>
  );
};

export default UserSettingPassword;
