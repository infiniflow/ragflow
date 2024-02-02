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
  const [form] = Form.useForm();
  const kFModel = useSelector((state: any) => state.kFModel);
  const { isShowCEFwModal } = kFModel;
  const { t } = useTranslation();

  const handleCancel = () => {
    dispatch({
      type: 'kFModel/updateState',
      payload: {
        isShowCEFwModal: false,
      },
    });
  };

  const createDocument = async () => {
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

  const handleOk = async () => {
    createDocument();
  };

  return (
    <Modal
      title="File Name"
      open={isShowCEFwModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Form
        form={form}
        name="validateOnly"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
      >
        <Form.Item<FieldType>
          label="File Name"
          name="name"
          rules={[{ required: true, message: 'Please input name!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default FileCreatingModal;
