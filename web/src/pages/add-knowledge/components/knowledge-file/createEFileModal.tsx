import { Form, Input, Modal } from 'antd';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { useDispatch, useSelector } from 'umi';

type FieldType = {
  name?: string;
};
interface kFProps {
  getKfList: () => void;
  kb_id: string;
}

const FileCreatingModal: React.FC<kFProps> = ({ getKfList, kb_id }) => {
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const { isShowCEFwModal } = kFModel;
  const [form] = Form.useForm();
  const { t } = useTranslation();

  const handleCancel = () => {
    dispatch({
      type: 'kFModel/updateState',
      payload: {
        isShowCEFwModal: false,
      },
    });
  };
  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      const retcode = await dispatch<any>({
        type: 'kFModel/document_create',
        payload: {
          name: values.name,
          kb_id,
        },
      });
      if (retcode === 0) {
        getKfList && getKfList();
      }
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <Modal
      title="Basic Modal"
      open={isShowCEFwModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Form
        form={form}
        name="validateOnly"
        labelCol={{ span: 8 }}
        wrapperCol={{ span: 16 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
      >
        <Form.Item<FieldType>
          label="文件名"
          name="name"
          rules={[{ required: true, message: 'Please input value!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default FileCreatingModal;
