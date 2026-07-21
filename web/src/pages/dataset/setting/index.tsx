import { DataFlowSelect } from '@/components/data-pipeline-select';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { FormLayout } from '@/constants/form';
import { ParseType } from '@/constants/knowledge';
import {
  BuiltinPipelineItem,
  ParseTypeItem,
} from '@/pages/dataset/dataset-setting/configuration/common-item';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useEffect, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import LinkDataSource, {
  IDataSourceNodeProps,
} from './components/link-data-source';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import {
  useActiveTab,
  useConnectorHandlers,
  useFetchDatasetSettingOnMount,
  usePipelineDataList,
  usePipelineOperatorNodes,
  useResetParserConfigOnPipelineChange,
  useSaveDatasetSetting,
} from './hooks';
import PipelineOperatorTabs from './pipeline-operator-tabs';

export default function DatasetSetting() {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      parse_type: ParseType.BuiltIn,
      pipeline_id: '',
      pipeline_name: '',
      pipeline_avatar: '',
      parser_id: '',
      parser_config: {},
      name: '',
      description: '',
      avatar: null,
      permission: '',
      embedding_model: '',
      pagerank: 0,
      connectors: [],
    },
  });

  const {
    knowledgeDetails,
    loading: datasetSettingLoading,
    sourceData,
  } = useFetchDatasetSettingOnMount(form);
  const { handleSave, loading: saveLoading } = useSaveDatasetSetting();

  const [sourceDataState, setSourceDataState] =
    useState<IDataSourceNodeProps[]>();

  useEffect(() => {
    setSourceDataState(sourceData);
  }, [sourceData]);

  const parseType = useWatch({
    control: form.control,
    name: 'parse_type',
    defaultValue: ParseType.BuiltIn,
  });
  const pipelineId = useWatch({
    control: form.control,
    name: 'pipeline_id',
    defaultValue: '',
  });

  const builtinPipelineId = useWatch({
    control: form.control,
    name: 'parser_id',
    defaultValue: '',
  });

  const pipelineParserConfig = knowledgeDetails?.parser_config as
    | Record<string, any>
    | undefined;

  const isPipelineMode = parseType === ParseType.Pipeline;
  const selectedPipelineId = isPipelineMode ? pipelineId : builtinPipelineId;
  // The saved parser_config only belongs to the pipeline (and parse type) it
  // was saved with — expose its id only while that exact pipeline is selected,
  // so that after switching, defaults come purely from the new pipeline's DSL.
  const savedParseType = knowledgeDetails?.pipeline_id
    ? ParseType.Pipeline
    : ParseType.BuiltIn;
  const savedPipelineId =
    parseType === savedParseType
      ? isPipelineMode
        ? knowledgeDetails?.pipeline_id
        : knowledgeDetails?.parser_id
      : undefined;

  const { operatorNodes, loading: operatorNodesLoading } =
    usePipelineOperatorNodes(
      selectedPipelineId,
      selectedPipelineId && selectedPipelineId === savedPipelineId
        ? pipelineParserConfig
        : undefined,
      !isPipelineMode,
    );

  useResetParserConfigOnPipelineChange(
    form,
    selectedPipelineId,
    savedPipelineId,
    operatorNodes,
  );

  const { activeTab, setActiveTab } = useActiveTab(operatorNodes);

  useEffect(() => {
    if (parseType === ParseType.BuiltIn) {
      form.setValue('pipeline_id', '');
      form.setValue('pipeline_name', '');
      form.setValue('pipeline_avatar', '');
    }
  }, [parseType, form]);

  const handleSubmit = useCallback(
    async (data: z.infer<typeof formSchema>) => {
      await handleSave(data);
    },
    [handleSave],
  );

  const { handleLinkOrEditSubmit, unbindFunc, handleAutoParse } =
    useConnectorHandlers(form, sourceDataState, setSourceDataState);

  const handleOperatorValuesChange = useCallback(
    (operatorId: string, values: any) => {
      const currentParserConfig = form.getValues('parser_config') || {};
      form.setValue('parser_config', {
        ...currentParserConfig,
        [operatorId]: values,
      });
    },
    [form],
  );

  const pipelineDataList = usePipelineDataList(sourceDataState);

  const showOperatorTabs =
    operatorNodes.length > 0 &&
    ((parseType === ParseType.Pipeline && !!pipelineId) ||
      (parseType === ParseType.BuiltIn && !!builtinPipelineId));

  return (
    <div className="pr-5 pb-5">
      <Card className="p-0 h-full flex flex-col bg-transparent shadow-none">
        <CardHeader className="p-5 border-b-0.5 border-border-button">
          <header>
            <CardTitle as="h1">
              {t('knowledgeDetails.nextConfiguration')}
            </CardTitle>
            <CardDescription>
              {t('knowledgeConfiguration.titleDescription')}
            </CardDescription>
          </header>
        </CardHeader>

        <CardContent className="p-0 flex-1 h-0 flex">
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(handleSubmit)}
              className="flex flex-col"
            >
              <div className="flex-1 h-0 w-[768px] px-5 pt-5 overflow-y-auto scrollbar-auto">
                <section className="space-y-5 text-text-secondary">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <GeneralForm></GeneralForm>

                  <Divider />
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.dataPipeline')}
                  </div>
                  <ParseTypeItem line={1} name="parse_type" />
                  {parseType === ParseType.BuiltIn && (
                    <BuiltinPipelineItem line={1} name="parser_id" />
                  )}
                  {parseType === ParseType.Pipeline && (
                    <DataFlowSelect
                      isMult={false}
                      showToDataPipeline={true}
                      formFieldName="pipeline_id"
                      layout={FormLayout.Horizontal}
                    />
                  )}
                  {showOperatorTabs && (
                    <PipelineOperatorTabs
                      nodes={operatorNodes}
                      value={activeTab}
                      onValueChange={setActiveTab}
                      onOperatorValuesChange={handleOperatorValuesChange}
                    />
                  )}

                  <Divider />
                  <LinkDataSource
                    data={pipelineDataList}
                    handleLinkOrEditSubmit={handleLinkOrEditSubmit}
                    unbindFunc={unbindFunc}
                    handleAutoParse={handleAutoParse}
                  />
                </section>
              </div>

              <div className="p-5 text-right items-center flex justify-end gap-3 w-[768px]">
                <Button
                  type="reset"
                  variant="transparent"
                  onClick={() => {
                    form.reset();
                  }}
                >
                  {t('knowledgeConfiguration.cancel')}
                </Button>

                <ButtonLoading
                  type="submit"
                  loading={
                    datasetSettingLoading ||
                    saveLoading ||
                    (operatorNodesLoading && operatorNodes.length === 0)
                  }
                >
                  {t('knowledgeConfiguration.save')}
                </ButtonLoading>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
