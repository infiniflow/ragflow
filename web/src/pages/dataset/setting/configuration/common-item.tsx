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
import { ArrowUpRight } from 'lucide-react';
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
              required
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
              required
              tooltip={t('embeddingModelTip')}
              className="text-sm  whitespace-wrap w-1/4"
            >
              {t('embeddingModel')}
            </FormLabel>
            <div className="text-muted-foreground w-3/4">
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

export function DataFlowItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'data_flow'}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="">
            <div className="flex gap-2 justify-between ">
              <FormLabel
                tooltip={t('dataFlowTip')}
                className="text-sm text-text-primary whitespace-wrap "
              >
                {t('dataFlow')}
              </FormLabel>
              <div className="text-sm flex text-text-primary">
                {t('buildItFromScratch')}
                <ArrowUpRight size={14} />
              </div>
            </div>

            <div className="text-muted-foreground">
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  placeholder={t('dataFlowPlaceholder')}
                  options={[{ value: '0', label: t('dataFlowDefault') }]}
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

export function DataExtractKnowledgeItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <>
      {' '}
      <FormField
        control={form.control}
        name={'extractKnowledgeGraph'}
        render={({ field }) => (
          <FormItem className=" items-center space-y-0 ">
            <div className="">
              <FormLabel
                tooltip={t('extractKnowledgeGraphTip')}
                className="text-sm  whitespace-wrap "
              >
                {t('extractKnowledgeGraph')}
              </FormLabel>
              <div className="text-muted-foreground">
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
      />{' '}
      <FormField
        control={form.control}
        name={'useRAPTORToEnhanceRetrieval'}
        render={({ field }) => (
          <FormItem className=" items-center space-y-0 ">
            <div className="">
              <FormLabel
                tooltip={t('useRAPTORToEnhanceRetrievalTip')}
                className="text-sm  whitespace-wrap "
              >
                {t('useRAPTORToEnhanceRetrieval')}
              </FormLabel>
              <div className="text-muted-foreground">
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
    </>
  );
}

export function TeamItem() {
  const { t } = useTranslate('knowledgeConfiguration');
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={'team'}
      render={({ field }) => (
        <FormItem className=" items-center space-y-0 ">
          <div className="">
            <FormLabel
              tooltip={t('teamTip')}
              className="text-sm  whitespace-wrap "
            >
              <span className="text-destructive mr-1"> *</span>
              {t('team')}
            </FormLabel>
            <div className="text-muted-foreground">
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  placeholder={t('teamPlaceholder')}
                  options={[{ value: '0', label: t('teamDefault') }]}
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
