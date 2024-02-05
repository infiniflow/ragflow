import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { Form, Input, Modal } from 'antd';
import { useDispatch, useNavigate, useSelector } from 'umi';

type FieldType = {
  name?: string;
};

const KnowledgeCreatingModal = ({
  visible,
  hideModal,
}: Omit<IModalManagerChildrenProps, 'showModal'>) => {
  const [form] = Form.useForm();
  const dispatch = useDispatch();
  const loading = useSelector(
    (state: any) => state.loading.effects['kSModel/createKb'],
  );
  const navigate = useNavigate();

  const handleOk = async () => {
    const ret = await form.validateFields();

    const data = await dispatch<any>({
      type: 'kSModel/createKb',
      payload: {
        name: ret.name,
      },
    });

    if (data.retcode === 0) {
      navigate(
        `/knowledge/${KnowledgeRouteKey.Configuration}?id=${data.data.kb_id}`,
      );
      hideModal();
    }
  };

  const handleCancel = () => {
    hideModal();
  };

  const onFinish = (values: any) => {
    console.log('Success:', values);
  };

  const onFinishFailed = (errorInfo: any) => {
    console.log('Failed:', errorInfo);
  };

  return (
    <Modal
      title="Create knowledge base"
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      okButtonProps={{ loading }}
    >
      <Form
        name="Create"
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

export default KnowledgeCreatingModal;
