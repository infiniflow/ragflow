import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import { Form, Input, Modal } from 'antd';
import { useEffect } from 'react';
import { useDispatch, useSelector } from 'umi';

const RenameModal = () => {
  const [form] = Form.useForm();
  const dispatch = useDispatch();
  const kFModel = useSelector((state: any) => state.kFModel);
  const loading = useSelector(
    (state: any) => state.loading.effects['kFModel/document_rename'],
  );
  const knowledgeBaseId = useKnowledgeBaseId();
  const isModalOpen = kFModel.isShowRenameModal;
  const initialName = kFModel.currentRecord?.name;
  const documentId = kFModel.currentRecord?.id;

  type FieldType = {
    name?: string;
  };

  const closeModal = () => {
    dispatch({
      type: 'kFModel/setIsShowRenameModal',
      payload: false,
    });
  };

  const handleOk = async () => {
    const ret = await form.validateFields();

    dispatch({
      type: 'kFModel/document_rename',
      payload: {
        doc_id: documentId,
        name: ret.name,
        kb_id: knowledgeBaseId,
      },
    });
  };

  const handleCancel = () => {
    closeModal();
  };

  const onFinish = (values: any) => {
    console.log('Success:', values);
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  useEffect(() => {
    form.setFieldValue('name', initialName);
  }, [initialName, documentId, form]);

  return (
    <Modal
      title="Rename"
      open={isModalOpen}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ loading }}
    >
      <Form
        name="basic"
        labelCol={{ span: 4 }}
        wrapperCol={{ span: 20 }}
        style={{ maxWidth: 600 }}
        onFinish={onFinish}
        onFinishFailed={onFinishFailed}
        autoComplete="off"
        form={form}
      >
        <Form.Item<FieldType>
          label="Name"
          name="name"
          rules={[{ required: true, message: 'Please input name!' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default RenameModal;
