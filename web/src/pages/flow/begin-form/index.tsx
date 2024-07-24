import { useTranslate } from '@/hooks/common-hooks';
import { Form, Input } from 'antd';
import { IOperatorForm } from '../interface';

type FieldType = {
  prologue?: string;
};

const BeginForm = ({ onValuesChange, form }: IOperatorForm) => {
  const { t } = useTranslate('chat');

  return (
    <Form
      name="basic"
      labelCol={{ span: 8 }}
      wrapperCol={{ span: 16 }}
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
