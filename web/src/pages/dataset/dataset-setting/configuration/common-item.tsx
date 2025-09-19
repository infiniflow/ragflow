import { SelectWithSearch } from '@/components/originui/select-with-search';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Radio } from '@/components/ui/radio';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { useFormContext } from 'react-hook-form';
import {
  useHasParsedDocument,
  useSelectChunkMethodList,
  useSelectEmbeddingModelOptions,
} from '../hooks';
interface IProps {
  line?: 1 | 2;
  isEdit?: boolean;
}
export function ChunkMethodItem(props: IProps) {
  const { line } = props;
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  // const handleChunkMethodSelectChange = useHandleChunkMethodSelectChange(form);
  const parserList = useSelectChunkMethodList();

  return (
    <FormField
      control={form.control}
      name={'parser_id'}
      render={({ field }) => (
        <FormItem className=" items-center space-y-1">
          <div className={line === 1 ? 'flex items-center' : ''}>
            <FormLabel
              required
              tooltip={t('chunkMethodTip')}
              className={cn('text-sm', {
                'w-1/4 whitespace-pre-wrap': line === 1,
              })}
            >
              {t('chunkMethod')}
            </FormLabel>
            <div className={line === 1 ? 'w-3/4 ' : 'w-full'}>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={parserList}
                  placeholder={t('chunkMethodPlaceholder')}
                />
              </FormControl>
            </div>
          </div>
          <div className="flex pt-1">
            <div className={line === 1 ? 'w-1/4' : ''}></div>
            <FormMessage />
          </div>
        </FormItem>
      )}
    />
  );
}
export function EmbeddingModelItem({ line = 1, isEdit = true }: IProps) {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();
  const embeddingModelOptions = useSelectEmbeddingModelOptions();
  const disabled = useHasParsedDocument(isEdit);
  return (
    <>
      <FormField
        control={form.control}
        name={'embd_id'}
        render={({ field }) => (
          <FormItem className={cn(' items-center space-y-0 ')}>
            <div
              className={cn('flex', {
                ' items-center': line === 1,
                'flex-col gap-1': line === 2,
              })}
            >
              <FormLabel
                required
                tooltip={t('embeddingModelTip')}
                className={cn('text-sm  whitespace-wrap ', {
                  'w-1/4': line === 1,
                })}
              >
                {t('embeddingModel')}
              </FormLabel>
              <div
                className={cn('text-muted-foreground', { 'w-3/4': line === 1 })}
              >
                <FormControl>
                  <SelectWithSearch
                    onChange={field.onChange}
                    value={field.value}
                    options={embeddingModelOptions}
                    disabled={isEdit ? disabled : false}
                    placeholder={t('embeddingModelPlaceholder')}
                  />
                </FormControl>
              </div>
            </div>
            <div className="flex pt-1">
              <div className={line === 1 ? 'w-1/4' : ''}></div>
              <FormMessage />
            </div>
          </FormItem>
        )}
      />
    </>
  );
}

export function ParseTypeItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'parseType'}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="">
            <FormLabel
              tooltip={t('parseTypeTip')}
              className="text-sm  whitespace-wrap "
            >
              {t('parseType')}
            </FormLabel>
            <div className="text-muted-foreground">
              <FormControl>
                <Radio.Group {...field}>
                  <div className="w-3/4 flex gap-2 justify-between text-muted-foreground">
                    <Radio value={1}>{t('builtIn')}</Radio>
                    <Radio value={2}>{t('manualSetup')}</Radio>
                  </div>
                </Radio.Group>
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

export function EnableAutoGenerateItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'enableAutoGenerate'}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="flex items-center">
            <FormLabel
              tooltip={t('enableAutoGenerateTip')}
              className="text-sm  whitespace-wrap w-1/4"
            >
              {t('enableAutoGenerate')}
            </FormLabel>
            <div className="text-muted-foreground w-3/4">
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
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
