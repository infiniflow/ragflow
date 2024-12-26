import { DelimiterInput } from '@/components/delimiter';
import { Form } from 'antd';
import { useTranslation } from 'react-i18next';
import { IOperatorForm } from '../../interface';
import DynamicInputVariable from '../components/dynamic-input-variable';

const IterationForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslation();

  return (
    <Form
      name="basic"
      autoComplete="off"
      form={form}
      onValuesChange={onValuesChange}
      layout={'vertical'}
    >
      <DynamicInputVariable nodeId={node?.id}></DynamicInputVariable>
      <Form.Item
        name={['delimiter']}
        label={t('knowledgeDetails.delimiter')}
        initialValue={`\\n!?;。；！？`}
        rules={[{ required: true }]}
        tooltip={t('knowledgeDetails.delimiterTip')}
      >
        <DelimiterInput maxLength={1} />
      </Form.Item>
    </Form>
  );
};

export default IterationForm;
