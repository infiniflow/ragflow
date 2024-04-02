import { IModalManagerChildrenProps } from '@/components/modal-manager';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { Form, Input, Modal } from 'antd';
import { useTranslation } from 'react-i18next';
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
  const { t } = useTranslation('translation', { keyPrefix: 'knowledgeList' });

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
      title={t('createKnowledgeBase')}
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
          label={t('name')}
          name="name"
          rules={[{ required: true, message: t('namePlaceholder') }]}
        >
          <Input placeholder={t('namePlaceholder')} />
        </Form.Item>
      </Form>
    </Modal>
  );
};

export default KnowledgeCreatingModal;
