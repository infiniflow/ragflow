import {
  LlmModelType,
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { Form, Select } from 'antd';
import camelCase from 'lodash/camelCase';

import { useTranslate } from '@/hooks/common-hooks';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { useCallback } from 'react';

interface IProps {
  prefix?: string;
  formItemLayout?: any;
  handleParametersChange?(value: ModelVariableType): void;
}

const LlmSettingItems = ({ prefix, formItemLayout = {} }: IProps) => {
  const form = Form.useFormInstance();
  const { t } = useTranslate('chat');
  const parameterOptions = Object.values(ModelVariableType).map((x) => ({
    label: t(camelCase(x)),
    value: x,
  }));

  const handleParametersChange = useCallback(
    (value: ModelVariableType) => {
      const variable = settledModelVariableMap[value];
      let nextVariable: Record<string, any> = variable;
      if (prefix) {
        nextVariable = { [prefix]: variable };
      }
      form.setFieldsValue(nextVariable);
    },
    [form, prefix],
  );

  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Image2text,
  ]);

  return (
    <>
      <Form.Item
        label={t('model')}
        name="llm_id"
        tooltip={t('modelTip')}
        {...formItemLayout}
        rules={[{ required: true, message: t('modelMessage') }]}
      >
        <Select
          options={modelOptions}
          showSearch
          popupMatchSelectWidth={false}
        />
      </Form.Item>
    </>
  );
};

export default LlmSettingItems;
