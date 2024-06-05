import { useTranslate } from '@/hooks/commonHooks';
import type { FormProps } from 'antd';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../interface';

type FieldType = {
  prologue?: string;
};

const onFinish: FormProps<FieldType>['onFinish'] = (values) => {
  console.log('Success:', values);
};

const onFinishFailed: FormProps<FieldType>['onFinishFailed'] = (errorInfo) => {
  console.log('Failed:', errorInfo);
};

const BeginForm = ({ onValuesChange }: IOperatorForm) => {
  const { t } = useTranslate('chat');
  const [form] = Form.useForm();

  return (
    <Form
      name="basic"
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
      style={{ maxWidth: 600 }}
      initialValues={{ remember: true }}
      onFinish={onFinish}
      onFinishFailed={onFinishFailed}
      onValuesChange={onValuesChange}
      autoComplete="off"
      form={form}
    >
      <Form.Item<FieldType>
        name={'prologue'}
        label={t('setAnOpener')}
        tooltip={t('setAnOpenerTip')}
        initialValue={t('setAnOpenerInitial')}
      >
        <Input.TextArea autoSize={{ minRows: 5 }} />
      </Form.Item>
    </Form>
  );
};

export default BeginForm;
