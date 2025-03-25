import { useTranslate } from '@/hooks/common-hooks';
import { useFetchFlow, useSettingFlow } from '@/hooks/flow-hooks';
import { normFile } from '@/utils/file-util';
import { PlusOutlined } from '@ant-design/icons';
import { Form, Input, Modal, Radio, Upload } from 'antd';
import React, { useCallback, useEffect } from 'react';
export function useFlowSettingModal() {
  const [visibleSettingModal, setVisibleSettingMModal] = React.useState(false);

  return {
    visibleSettingModal,
    setVisibleSettingMModal,
  };
}

type FlowSettingModalProps = {
  visible: boolean;
  hideModal: () => void;
  id: string;
};
export const FlowSettingModal = ({
  hideModal,
  visible,
  id,
}: FlowSettingModalProps) => {
  const { data, refetch } = useFetchFlow();
  const [form] = Form.useForm();
  const { t } = useTranslate('flow.settings');
  const { loading, settingFlow } = useSettingFlow();
  // Initialize form with data when it becomes available
  useEffect(() => {
    if (data) {
      form.setFieldsValue({
        title: data.title,
        description: data.description,
        permission: data.permission,
        avatar: data.avatar ? [{ thumbUrl: data.avatar }] : [],
      });
    }
  }, [data, form]);

  const handleSubmit = useCallback(async () => {
    if (!id) return;
    try {
      const { avatar, ...others } = await form.validateFields();
      const param = {
        ...others,
        id,
        avatar: avatar && avatar.length > 0 ? avatar[0].thumbUrl : '',
      };
      settingFlow(param);
    } catch (error) {
      console.error('Validation failed:', error);
    }
  }, [form, id, settingFlow]);
  React.useEffect(() => {
    if (!loading && refetch && visible) {
      refetch();
    }
  }, [loading, refetch, visible]);
  return (
    <Modal
      confirmLoading={loading}
      title={t('agentSetting')}
      open={visible}
      onCancel={hideModal}
      onOk={handleSubmit}
      okText={t('save', { keyPrefix: 'common' })}
      cancelText={t('cancel', { keyPrefix: 'common' })}
    >
      <Form
        form={form}
        labelCol={{ span: 6 }}
        wrapperCol={{ span: 18 }}
        layout="horizontal"
        style={{ maxWidth: 600 }}
      >
        <Form.Item
          name="title"
          label={t('title')}
          rules={[{ required: true, message: 'Please input a title!' }]}
        >
          <Input />
        </Form.Item>
        <Form.Item
          name="avatar"
          label={t('photo')}
          valuePropName="fileList"
          getValueFromEvent={normFile}
        >
          <Upload
            listType="picture-card"
            maxCount={1}
            beforeUpload={() => false}
            showUploadList={{ showPreviewIcon: false, showRemoveIcon: false }}
          >
            <button style={{ border: 0, background: 'none' }} type="button">
              <PlusOutlined />
              <div style={{ marginTop: 8 }}>{t('upload')}</div>
            </button>
          </Upload>
        </Form.Item>
        <Form.Item name="description" label={t('description')}>
          <Input.TextArea rows={4} />
        </Form.Item>

        <Form.Item
          name="permission"
          label={t('permissions')}
          tooltip={t('permissionsTip')}
          rules={[{ required: true }]}
        >
          <Radio.Group>
            <Radio value="me">{t('me')}</Radio>
            <Radio value="team">{t('team')}</Radio>
          </Radio.Group>
        </Form.Item>
      </Form>
    </Modal>
  );
};
