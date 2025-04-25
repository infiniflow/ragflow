import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { DocumentParserType } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/use-knowledge-request';
import { IModalProps } from '@/interfaces/common';
import { IParserConfig } from '@/interfaces/database/document';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { zodResolver } from '@hookform/resolvers/zod';
import {} from 'module';
import { useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import {
  AutoKeywordsFormField,
  AutoQuestionsFormField,
} from '../auto-keywords-form-field';
import { DatasetConfigurationContainer } from '../dataset-configuration-container';
import { DelimiterFormField } from '../delimiter-form-field';
import { EntityTypesFormField } from '../entity-types-form-field';
import { ExcelToHtmlFormField } from '../excel-to-html-form-field';
import {
  DocumentType,
  LayoutRecognizeFormField,
} from '../layout-recognize-form-field';
import { MaxTokenNumberFormField } from '../max-token-number-from-field';
import {
  UseGraphRagFormField,
  showGraphRagItems,
} from '../parse-configuration/graph-rag-form-fields';
import RaptorFormFields, {
  showRaptorParseConfiguration,
} from '../parse-configuration/raptor-form-fields';
import { Input } from '../ui/input';
import { RAGFlowSelect } from '../ui/select';
import { useFetchParserListOnMount, useShowAutoKeywords } from './hooks';

const FormId = 'ChunkMethodDialogForm';

interface IProps
  extends IModalProps<{
    parserId: string;
    parserConfig: IChangeParserConfigRequestBody;
  }> {
  loading: boolean;
  parserId: string;
  parserConfig: IParserConfig;
  documentExtension: string;
  documentId: string;
}

const hidePagesChunkMethods = [
  DocumentParserType.Qa,
  DocumentParserType.Table,
  DocumentParserType.Picture,
  DocumentParserType.Resume,
  DocumentParserType.One,
  DocumentParserType.KnowledgeGraph,
];

export function ChunkMethodDialog({
  hideModal,
  onOk,
  parserId,
  documentId,
  documentExtension,
}: IProps) {
  const { t } = useTranslate('knowledgeDetails');

  const { parserList } = useFetchParserListOnMount(
    documentId,
    parserId,
    documentExtension,
    // form,
  );

  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration();

  const useGraphRag = useMemo(() => {
    return knowledgeDetails.parser_config?.graphrag?.use_graphrag;
  }, [knowledgeDetails.parser_config?.graphrag?.use_graphrag]);

  const FormSchema = z.object({
    parser_id: z
      .string()
      .min(1, {
        message: 'namePlaceholder',
      })
      .trim(),
    parser_config: z.object({
      task_page_size: z.coerce.number(),
      layout_recognize: z.string(),
    }),
  });
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      parser_id: parserId,
      parser_config: {
        task_page_size: 12,
        layout_recognize: DocumentType.DeepDOC,
      },
    },
  });

  const layoutRecognize = useWatch({
    name: 'parser_config.layout_recognize',
    control: form.control,
  });

  const selectedTag = useWatch({
    name: 'parser_id',
    control: form.control,
  });

  const isPdf = documentExtension === 'pdf';

  const showPages = useMemo(() => {
    return isPdf && hidePagesChunkMethods.every((x) => x !== selectedTag);
  }, [selectedTag, isPdf]);

  const showOne = useMemo(() => {
    return (
      isPdf &&
      hidePagesChunkMethods
        .filter((x) => x !== DocumentParserType.One)
        .every((x) => x !== selectedTag)
    );
  }, [selectedTag, isPdf]);

  const showMaxTokenNumber =
    selectedTag === DocumentParserType.Naive ||
    selectedTag === DocumentParserType.KnowledgeGraph;

  const showEntityTypes = selectedTag === DocumentParserType.KnowledgeGraph;

  const showExcelToHtml =
    selectedTag === DocumentParserType.Naive && documentExtension === 'xlsx';

  const showAutoKeywords = useShowAutoKeywords();

  async function onSubmit(data: z.infer<typeof FormSchema>) {
    console.log('🚀 ~ onSubmit ~ data:', data);
    // const ret = await onOk?.();
    // if (ret) {
    //   hideModal?.();
    // }
  }

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-w-[50vw]">
        <DialogHeader>
          <DialogTitle>{t('chunkMethod')}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6"
            id={FormId}
          >
            <FormField
              control={form.control}
              name="parser_id"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('name')}</FormLabel>
                  <FormControl>
                    <RAGFlowSelect
                      {...field}
                      options={parserList}
                    ></RAGFlowSelect>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {showPages && layoutRecognize && (
              <FormField
                control={form.control}
                name="parser_config.task_page_size"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel tooltip={t('taskPageSizeTip')}>
                      {t('taskPageSize')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type={'number'}
                        min={1}
                        max={128}
                      ></Input>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            <DatasetConfigurationContainer show={showOne || showMaxTokenNumber}>
              {showOne && <LayoutRecognizeFormField></LayoutRecognizeFormField>}
              {showMaxTokenNumber && (
                <>
                  <MaxTokenNumberFormField
                    max={
                      selectedTag === DocumentParserType.KnowledgeGraph
                        ? 8192 * 2
                        : 2048
                    }
                  ></MaxTokenNumberFormField>
                  <DelimiterFormField></DelimiterFormField>
                </>
              )}
            </DatasetConfigurationContainer>
            <DatasetConfigurationContainer
              show={showAutoKeywords(selectedTag) || showExcelToHtml}
            >
              {showAutoKeywords(selectedTag) && (
                <>
                  <AutoKeywordsFormField></AutoKeywordsFormField>
                  <AutoQuestionsFormField></AutoQuestionsFormField>
                </>
              )}
              {showExcelToHtml && <ExcelToHtmlFormField></ExcelToHtmlFormField>}
            </DatasetConfigurationContainer>
            {showRaptorParseConfiguration(
              selectedTag as DocumentParserType,
            ) && (
              <DatasetConfigurationContainer>
                <RaptorFormFields></RaptorFormFields>
              </DatasetConfigurationContainer>
            )}
            {showGraphRagItems(selectedTag as DocumentParserType) &&
              useGraphRag && <UseGraphRagFormField></UseGraphRagFormField>}
            {showEntityTypes && <EntityTypesFormField></EntityTypesFormField>}
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form={FormId}>
            Save changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
