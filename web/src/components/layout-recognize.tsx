import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { Form, Select } from 'antd';
import { useMemo } from 'react';

const enum DocumentType {
  DeepDOC = 'DeepDOC',
  PlainText = 'Plain Text',
}

const LayoutRecognize = () => {
  const { t } = useTranslate('knowledgeDetails');
  const allOptions = useSelectLlmOptionsByModelType();

  const options = useMemo(() => {
    const list = [DocumentType.DeepDOC, DocumentType.PlainText].map((x) => ({
      label: x,
      value: x,
    }));

    return [...list, ...allOptions[LlmModelType.Image2text]];
  }, [allOptions]);

  return (
    <Form.Item
      name={['parser_config', 'layout_recognize']}
      label={t('layoutRecognize')}
      initialValue={DocumentType.DeepDOC}
      tooltip={t('layoutRecognizeTip')}
    >
      <Select options={options} />
    </Form.Item>
  );
};

export default LayoutRecognize;
