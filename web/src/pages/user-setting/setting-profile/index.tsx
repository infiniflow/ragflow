import {
  useFetchUserInfo,
  useSaveSetting,
  useSelectUserInfo,
} from '@/hooks/userSettingHook';
import {
  getBase64FromUploadFileList,
  getUploadFileListFromBase64,
  normFile,
} from '@/utils/fileUtil';
import { PlusOutlined } from '@ant-design/icons';
import {
  Button,
  Divider,
  Form,
  Input,
  Select,
  Space,
  Spin,
  Upload,
  UploadFile,
} from 'antd';
import { useEffect } from 'react';
import SettingTitle from '../components/setting-title';
import { TimezoneList } from '../constants';
import {
  useSelectSubmitUserInfoLoading,
  useSelectUserInfoLoading,
  useValidateSubmittable,
} from '../hooks';

import parentStyles from '../index.less';
import styles from './index.less';

const { Option } = Select;

type FieldType = {
  nickname?: string;
  language?: string;
  email?: string;
  color_schema?: string;
  timezone?: string;
  avatar?: string;
};

const tailLayout = {
  wrapperCol: { offset: 20, span: 4 },
};

const UserSettingProfile = () => {
  const userInfo = useSelectUserInfo();
  const saveSetting = useSaveSetting();
  const submitLoading = useSelectSubmitUserInfoLoading();
  const { form, submittable } = useValidateSubmittable();
  const loading = useSelectUserInfoLoading();
  useFetchUserInfo();

  const onFinish = async (values: any) => {
    const avatar = await getBase64FromUploadFileList(values.avatar);
    saveSetting({ ...values, avatar });
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  useEffect(() => {
    const fileList: UploadFile[] = getUploadFileListFromBase64(userInfo.avatar);
    form.setFieldsValue({ ...userInfo, avatar: fileList });
  }, [form, userInfo]);

  return (
    <section className={styles.profileWrapper}>
      <SettingTitle
        title="Profile"
        description="Update your photo and personal details here."
      ></SettingTitle>
      <Divider />
      <Spin spinning={loading}>
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
        >
          <Form.Item<FieldType>
            label="Username"
            name="nickname"
            rules={[
              {
                required: true,
                message: 'Please input your username!',
                whitespace: true,
              },
            ]}
          >
            <Input />
          </Form.Item>
          <Divider />
          <Form.Item<FieldType>
            label={
              <div>
                <Space>Your photo</Space>
                <div>This will be displayed on your profile.</div>
              </div>
            }
            name="avatar"
            valuePropName="fileList"
            getValueFromEvent={normFile}
          >
            <Upload
              listType="picture-card"
              maxCount={1}
              accept="image/*"
              beforeUpload={() => {
                return false;
              }}
              showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
            >
              <button style={{ border: 0, background: 'none' }} type="button">
                <PlusOutlined />
                <div style={{ marginTop: 8 }}>Upload</div>
              </button>
            </Upload>
          </Form.Item>
          <Divider />
          <Form.Item<FieldType>
            label="Color schema"
            name="color_schema"
            rules={[
              { required: true, message: 'Please select your color schema!' },
            ]}
          >
            <Select placeholder="select your color schema">
              <Option value="Bright">Bright</Option>
              <Option value="Dark">Dark</Option>
            </Select>
          </Form.Item>
          <Divider />
          <Form.Item<FieldType>
            label="Language"
            name="language"
            rules={[{ required: true, message: 'Please input your language!' }]}
          >
            <Select placeholder="select your language">
              <Option value="English">English</Option>
              <Option value="Chinese">Chinese</Option>
            </Select>
          </Form.Item>
          <Divider />
          <Form.Item<FieldType>
            label="Timezone"
            name="timezone"
            rules={[{ required: true, message: 'Please input your timezone!' }]}
          >
            <Select placeholder="select your timezone" showSearch>
              {TimezoneList.map((x) => (
                <Option value={x} key={x}>
                  {x}
                </Option>
              ))}
            </Select>
          </Form.Item>
          <Divider />
          <Form.Item label="Email address">
            <Form.Item<FieldType> name="email" noStyle>
              <Input disabled />
            </Form.Item>
            <p className={parentStyles.itemDescription}>
              Once registered, E-mail cannot be changed.
            </p>
          </Form.Item>
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
                loading={submitLoading}
              >
                Save
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Spin>
    </section>
  );
};

export default UserSettingProfile;
