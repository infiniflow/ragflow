import LLMSelect from '@/components/llm-select';
import { useTranslate } from '@/hooks/commonHooks';
import { Form, Select } from 'antd';
import { Operator } from '../constant';
import {
  useBuildFormSelectOptions,
  useHandleFormSelectChange,
} from '../form-hooks';
import { useSetLlmSetting } from '../hooks';
import { IOperatorForm } from '../interface';
import { useWatchConnectionChanges } from './hooks';

const RelevantForm = ({ onValuesChange, form, node }: IOperatorForm) => {
  const { t } = useTranslate('flow');
  useSetLlmSetting(form);
  const buildRelevantOptions = useBuildFormSelectOptions(
    Operator.Relevant,
    node?.id,
  );
  useWatchConnectionChanges({ nodeId: node?.id, form });
  const { handleSelectChange } = useHandleFormSelectChange(node?.id);

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
          onChange={handleSelectChange('yes')}
        />
      </Form.Item>
      <Form.Item label={t('no')} name={'no'}>
        <Select
          allowClear
          options={buildRelevantOptions([form?.getFieldValue('yes')])}
          onChange={handleSelectChange('no')}
        />
      </Form.Item>
    </Form>
  );
};

export default RelevantForm;
