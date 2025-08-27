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
        <FormItem className=" items-center space-y-0 ">
          <div className="flex items-center">
            <FormLabel
              tooltip={t('chunkMethodTip')}
              className="text-sm text-muted-foreground whitespace-wrap w-1/4"
            >
              {t('chunkMethod')}
            </FormLabel>
            <div className="w-3/4 ">
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={parserList}
                  placeholder={t('chunkMethodPlaceholder')}
                  // onChange={handleChunkMethodSelectChange}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className="w-1/4"></div>
            <FormMessage />
          </div>
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
        <FormItem className=" items-center space-y-0 ">
          <div className="flex items-center">
            <FormLabel
              tooltip={t('embeddingModelTip')}
              className="text-sm text-muted-foreground whitespace-wrap w-1/4"
            >
              {t('embeddingModel')}
            </FormLabel>
            <div className="w-3/4">
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={embeddingModelOptions}
                  disabled={disabled}
                  placeholder={t('embeddingModelPlaceholder')}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className="w-1/4"></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
