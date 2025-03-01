import { useTranslate } from '@/hooks/common-hooks';
import { useHandleChunkMethodSelectChange } from '@/hooks/logic-hooks';
import { Form, Select } from 'antd';
import { memo } from 'react';
import {
  useHasParsedDocument,
  useSelectChunkMethodList,
  useSelectEmbeddingModelOptions,
} from '../hooks';

export const EmbeddingModelItem = memo(function EmbeddingModelItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const embeddingModelOptions = useSelectEmbeddingModelOptions();
  const disabled = useHasParsedDocument();

  return (
    <Form.Item
      name="embd_id"
      label={t('embeddingModel')}
      rules={[{ required: true }]}
      tooltip={t('embeddingModelTip')}
    >
      <Select
        placeholder={t('embeddingModelPlaceholder')}
        options={embeddingModelOptions}
        disabled={disabled}
      ></Select>
    </Form.Item>
  );
});

export const ChunkMethodItem = memo(function ChunkMethodItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = Form.useFormInstance();
  const handleChunkMethodSelectChange = useHandleChunkMethodSelectChange(form);
  const disabled = useHasParsedDocument();
  const parserList = useSelectChunkMethodList();

  return (
    <Form.Item
      name="parser_id"
      label={t('chunkMethod')}
      tooltip={t('chunkMethodTip')}
      rules={[{ required: true }]}
    >
      <Select
        placeholder={t('chunkMethodPlaceholder')}
        disabled={disabled}
        onChange={handleChunkMethodSelectChange}
        options={parserList}
      ></Select>
    </Form.Item>
  );
});
