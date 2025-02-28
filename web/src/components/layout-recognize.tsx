import { LlmModelType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { Form, Select } from 'antd';
import { camelCase } from 'lodash';
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
      label: x === DocumentType.PlainText ? t(camelCase(x)) : 'DeepDoc',
      value: x,
    }));

    const image2TextList = allOptions[LlmModelType.Image2text].map((x) => {
      return {
        ...x,
        options: x.options.map((y) => {
          return {
            ...y,
            label: (
              <div className="flex justify-between items-center gap-2">
                {y.label}
                <span className="text-red-500 text-sm">Experimental</span>
              </div>
            ),
          };
        }),
      };
    });

    return [...list, ...image2TextList];
  }, [allOptions, t]);

  return (
    <Form.Item
      name={['parser_config', 'layout_recognize']}
      label={t('layoutRecognize')}
      initialValue={DocumentType.DeepDOC}
      tooltip={t('layoutRecognizeTip')}
    >
      <Select options={options} popupMatchSelectWidth={false} />
    </Form.Item>
  );
};

export default LayoutRecognize;
