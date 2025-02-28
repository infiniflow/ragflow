import LLMSelect from '@/components/llm-select';
import MessageHistoryWindowSizeItem from '@/components/message-history-window-size-item';
import { useTranslate } from '@/hooks/common-hooks';
import { Form, Select } from 'antd';
import { GoogleLanguageOptions } from '../../constant';
import { IOperatorForm } from '../../interface';

const RewriteQuestionForm = ({ onValuesChange, form }: IOperatorForm) => {
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
      <Form.Item
        name={'llm_id'}
        label={t('model', { keyPrefix: 'chat' })}
        tooltip={t('modelTip', { keyPrefix: 'chat' })}
      >
        <LLMSelect></LLMSelect>
      </Form.Item>
      <Form.Item
        label={t('language')}
        name={'language'}
        tooltip={t('languageTip')}
      >
        <Select options={GoogleLanguageOptions} allowClear={true}></Select>
      </Form.Item>
      <MessageHistoryWindowSizeItem
        initialValue={6}
      ></MessageHistoryWindowSizeItem>
    </Form>
  );
};

export default RewriteQuestionForm;
