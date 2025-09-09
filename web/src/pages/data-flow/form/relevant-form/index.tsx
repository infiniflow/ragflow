import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { Operator } from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';
import { IOperatorForm } from '../../interface';
import { useWatchConnectionChanges } from './hooks';

const RelevantForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  const buildRelevantOptions = useBuildFormSelectOptions(
    Operator.Relevant,
    node?.id,
  );
  useWatchConnectionChanges({ nodeId: node?.id, form });

  return (
    <Form
      name="basic"
      labelCol={{ span: 4 }}
      wrapperCol={{ span: 20 }}
      onValuesChange={onValuesChange}
      autoComplete="off"
      form={form}
    >
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <Form.Item label={t('yes')} name={'yes'}>
        <Select
          allowClear
          options={buildRelevantOptions([form?.getFieldValue('no')])}
        />
      </Form.Item>
      <Form.Item label={t('no')} name={'no'}>
        <Select
          allowClear
          options={buildRelevantOptions([form?.getFieldValue('yes')])}
        />
      </Form.Item>
    </Form>
  );
};

export default RelevantForm;
