import { Form, Input, Modal } from 'antd';
import { useTranslation } from 'react-i18next';
import { useDispatch, useSelector } from 'umi';

type FieldType = {
  api_key?: string;
};

const SakModal = () => {
  const dispatch = useDispatch();
  const settingModel = useSelector((state: any) => state.settingModel);
  const { isShowSAKModal, llm_factory } = settingModel;
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const handleCancel = () => {
    dispatch({
      type: 'settingModel/updateState',
      payload: {
        isShowSAKModal: false,
      },
    });
  };
  const handleOk = async () => {
    try {
      const values = await form.validateFields();

      dispatch({
        type: 'settingModel/set_api_key',
        payload: {
          api_key: values.api_key,
          llm_factory: llm_factory,
        },
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <Modal
      title="Basic Modal"
      open={isShowSAKModal}
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
          label="API Key"
          name="api_key"
          rules={[{ required: true, message: 'Please input ' }]}
        >
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default SakModal;
