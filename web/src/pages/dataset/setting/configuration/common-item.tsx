import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { useTranslate } from '@/hooks/common-hooks';
import { useFormContext } from 'react-hook-form';
import {
  useHasParsedDocument,
  useSelectChunkMethodList,
  useSelectEmbeddingModelOptions,
} from '../hooks';

export function ChunkMethodItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  // const handleChunkMethodSelectChange = useHandleChunkMethodSelectChange(form);
  const parserList = useSelectChunkMethodList();

  return (
    <FormField
      control={form.control}
      name={'parser_id'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('chunkMethodTip')}>
            {t('chunkMethod')}
          </FormLabel>
          <FormControl>
            <RAGFlowSelect
              {...field}
              options={parserList}
              placeholder={t('chunkMethodPlaceholder')}
              // onChange={handleChunkMethodSelectChange}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

export function EmbeddingModelItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const embeddingModelOptions = useSelectEmbeddingModelOptions();
  const disabled = useHasParsedDocument();

  return (
    <FormField
      control={form.control}
      name={'embd_id'}
      render={({ field }) => (
        <FormItem>
          <FormLabel tooltip={t('embeddingModelTip')}>
            {t('embeddingModel')}
          </FormLabel>
          <FormControl>
            <RAGFlowSelect
              {...field}
              options={embeddingModelOptions}
              disabled={disabled}
              placeholder={t('embeddingModelPlaceholder')}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
