import { rsaPsw } from '@/utils';
import { Form, Input, Modal } from 'antd';
import { useTranslation } from 'react-i18next';
import { useDispatch, useSelector } from 'umi';

type FieldType = {
  newPassword?: string;
  password?: string;
};

const CpwModal = () => {
  const dispatch = useDispatch();
  const settingModel = useSelector((state: any) => state.settingModel);
  const { isShowPSwModal } = settingModel;
  const { t } = useTranslation();
  const [form] = Form.useForm();

  const handleCancel = () => {
    dispatch({
      type: 'settingModel/updateState',
      payload: {
        isShowPSwModal: false,
      },
    });
  };
  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      var password = rsaPsw(values.password);
      var new_password = rsaPsw(values.newPassword);

      dispatch({
        type: 'settingModel/setting',
        payload: {
          password,
          new_password,
        },
      });
    } catch (errorInfo) {
      console.log('Failed:', errorInfo);
    }
  };

  return (
    <Modal
      title="Basic Modal"
      open={isShowPSwModal}
      onOk={handleOk}
      onCancel={handleCancel}
    >
      <Form
        form={form}
        labelCol={{ span: 8 }}
        wrapperCol={{ span: 16 }}
        style={{ maxWidth: 600 }}
        autoComplete="off"
      >
        <Form.Item<FieldType>
          label="旧密码"
          name="password"
          rules={[{ required: true, message: 'Please input value' }]}
        >
          <Input.Password />
        </Form.Item>
        <Form.Item<FieldType>
          label="新密码"
          name="newPassword"
          rules={[
            { required: true, message: 'Please input your newPassword!' },
          ]}
        >
          <Input.Password />
        </Form.Item>
      </Form>
    </Modal>
  );
};
export default CpwModal;
