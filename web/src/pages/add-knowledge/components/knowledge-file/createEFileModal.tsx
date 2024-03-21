import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { useFetchDocumentList } from '@/hooks/documentHooks';
import { useGetKnowledgeSearchParams } from '@/hooks/routeHook';
import { Form, Input, Modal } from 'antd';
import React from 'react';
import { useDispatch } from 'umi';

type FieldType = {
  name?: string;
};

interface IProps extends Omit<IModalManagerChildrenProps, 'showModal'> {
  loading: boolean;
  onOk: (name: string) => void;
  showModal?(): void;
}

const FileCreatingModal: React.FC<IProps> = ({ visible, hideModal }) => {
  const fetchKfList = useFetchDocumentList();
  const dispatch = useDispatch();
  const { knowledgeId } = useGetKnowledgeSearchParams();
  const [form] = Form.useForm();

  const createDocument = async () => {
    try {
      const values = await form.validateFields();
      const retcode = await dispatch<any>({
        type: 'kFModel/document_create',
        payload: {
          name: values.name,
          kb_id: knowledgeId,
        },
      });
      if (retcode === 0 && fetchKfList) {
        fetchKfList();
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
      open={visible}
      onOk={handleOk}
      onCancel={hideModal}
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
