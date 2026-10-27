import { ParserSelect } from '@/components/parser-select';
import PipelineOperatorTabs from '@/components/pipeline-operator-tabs';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import Divider from '@/components/ui/divider';
import { Modal } from '@/components/ui/modal/modal';
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { ParseType } from '@/constants/knowledge';
import { buildParserSelectValue } from '@/hooks/use-parser-options';
import { useParserSelectHandler } from '@/hooks/use-parser-select-handler';
import {
  useActiveTab,
  usePipelineOperatorNodes,
  useResetParserConfigOnPipelineChange,
} from '@/hooks/use-pipeline-operator';
import { buildParserConfigFromNodes } from '@/utils/pipeline-operator';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEqual } from 'lodash';
import { useCallback, useEffect, useState } from 'react';
import { FieldErrors, useForm, useFormState, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import ChunkMethodLearnMore from './chunk-method-learn-more';
import LinkDataSource, {
  IDataSourceNodeProps,
} from './components/link-data-source';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import {
  useConnectorHandlers,
  useFetchDatasetSettingOnMount,
  usePipelineDataList,
  useSaveDatasetSetting,
} from './hooks';
import { normalizeParserConfigForComparison } from './parser-config';
import { PipelineVersionSelect } from '@/components/pipeline-version-select';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';
import { useRevealSubmitErrors } from './use-reveal-submit-errors';

export default function DatasetSetting() {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      parse_type: ParseType.BuiltIn,
      pipeline_id: '',
      parser_id: '',
      parser_config: {},
      name: '',
      description: '',
      avatar: null,
      permission: '',
      language: 'English',
      embedding_model: '',
      pagerank: 0,
      connectors: [],
    },
  });

  const handleParserSelect = useParserSelectHandler(form);

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
  const pipelineVersionId = useWatch({
    control: form.control,
    name: 'pipeline_version_id',
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

  // Published versions of the selected dataflow canvas (design C1/C2).
  const { data: canvasVersions } = useFetchCanvasVersionList(
    parseType === ParseType.Pipeline && pipelineId ? pipelineId : undefined,
  );
  // Show the outdated bang only while the form still holds the SAVED pin:
  // once the user picks a different (unsaved) version the bang would be
  // misleading (design D3: hint only, never auto-upgrade).
  const savedVersionOutdated =
    !!knowledgeDetails?.pipeline_version_outdated &&
    (pipelineVersionId ?? '') === (knowledgeDetails?.pipeline_version_id ?? '');

  // Design section 8 (T37): an unpinned dataset must still pre-select the latest
  // concrete canvas version instead of the "Latest version" placeholder. We only
  // fill an empty field, so a saved pin or a user selection wins. This effect
  // runs after the on-mount form.reset (which sets an unpinned dataset to ''),
  // so the latest concrete id wins once the version list resolves.
  useEffect(() => {
    const savedPin = knowledgeDetails?.pipeline_version_id;
    if (
      !savedPin &&
      canvasVersions?.length &&
      !form.getValues('pipeline_version_id')
    ) {
      form.setValue('pipeline_version_id', canvasVersions[0].id);
    }
  }, [knowledgeDetails, canvasVersions, form]);

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

  // A pinned version belongs to the canvas it was pinned on (design section
  // 5/B4 sticky snapshot). Switching the dataflow canvas invalidates any
  // previously pinned version, so clear it; otherwise a version id from the
  // old canvas would be submitted against the new pipeline. The initial load
  // keeps the saved version (selected equals saved).
  useEffect(() => {
    if (selectedPipelineId !== savedPipelineId) {
      form.setValue('pipeline_version_id', '');
    }
  }, [selectedPipelineId, savedPipelineId, form]);

  const { activeTab, setActiveTab } = useActiveTab(operatorNodes);

  const { scrollContainerRef, handleInvalidSubmit } =
    useRevealSubmitErrors(setActiveTab);

  useEffect(() => {
    if (parseType === ParseType.BuiltIn) {
      form.setValue('pipeline_id', '');
    }
  }, [parseType, form]);

  const handleSubmit = useCallback(
    async (data: z.infer<typeof formSchema>) => {
      const savedValues = form.formState.defaultValues;
      const operatorDefaults = buildParserConfigFromNodes(operatorNodes);
      const [savedParserConfig, parserConfig] = [
        savedValues?.parser_config,
        data.parser_config,
      ].map((config) =>
        normalizeParserConfigForComparison(config, operatorDefaults, t),
      );
      const parserSettingsChanged =
        data.parse_type !== savedValues?.parse_type ||
        (data.parser_id || '') !== (savedValues?.parser_id || '') ||
        (data.pipeline_id || '') !== (savedValues?.pipeline_id || '') ||
        !isEqual(parserConfig, savedParserConfig);

      if (knowledgeDetails.chunk_count > 0 && parserSettingsChanged) {
        await new Promise<void>((resolve) => {
          Modal.warning({
            title: t('common.warn'),
            content: t('knowledgeConfiguration.parserSettingsChangeWarning'),
            okText: t('common.ok'),
            closable: false,
            showCancel: false,
            onOk: resolve,
          });
        });
      }

      const result = await handleSave(data);
      if (result.code === 0) {
        form.reset(data);
      }
    },
    [form, handleSave, knowledgeDetails.chunk_count, operatorNodes, t],
  );

  const { errors } = useFormState({
    control: form.control,
    name: 'parser_config',
  });

  const { handleLinkOrEditSubmit, unbindFunc, handleAutoParse } =
    useConnectorHandlers(form, sourceDataState, setSourceDataState);

  const handleOperatorValuesChange = useCallback(
    (operatorId: string, values: any) => {
      const currentParserConfig = form.getValues('parser_config') || {};
      // Skip no-op syncs (e.g. a remounted tab pushing back the values it was
      // just initialized with) — otherwise each setValue re-renders the tabs,
      // which re-fires the operator form's change callback in a loop.
      if (isEqual(currentParserConfig[operatorId], values)) {
        return;
      }
      form.setValue('parser_config', {
        ...currentParserConfig,
        [operatorId]: values,
      });
    },
    [form],
  );

  const parserConfigValues = useWatch({
    control: form.control,
    name: 'parser_config',
  });

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

        <CardContent className="p-0 flex-1 h-0 flex divide-x-0.5">
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(handleSubmit, handleInvalidSubmit)}
              className="flex flex-col"
            >
              <div
                ref={scrollContainerRef}
                className="flex-1 h-0 w-[768px] px-5 pt-5 overflow-y-auto scrollbar-auto"
              >
                <section className="space-y-5 text-text-secondary">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <GeneralForm></GeneralForm>

                  <Divider />
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.dataPipeline')}
                  </div>
                  <FormItem className="items-center space-y-0">
                    <div className="flex items-center gap-1">
                      <FormLabel className="text-sm text-text-secondary whitespace-wrap w-1/4">
                        {t('knowledgeConfiguration.parsingMode')}
                      </FormLabel>
                      <div className="text-muted-foreground w-3/4">
                        <FormField
                          control={form.control}
                          name={
                            parseType === ParseType.BuiltIn
                              ? 'parser_id'
                              : 'pipeline_id'
                          }
                          render={() => (
                            <FormItem className="space-y-1">
                              <ParserSelect
                                value={buildParserSelectValue({
                                  parseType,
                                  parserId: builtinPipelineId,
                                  pipelineId,
                                })}
                                onChange={handleParserSelect}
                              />
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </div>
                    </div>
                  </FormItem>
                  {parseType === ParseType.Pipeline && !!pipelineId && (
                    <FormItem className="items-center space-y-0">
                      <div className="flex items-center gap-1">
                        <FormLabel className="text-sm text-text-secondary whitespace-wrap w-1/4">
                          {t('flow.version.label')}
                        </FormLabel>
                        <div className="text-muted-foreground w-3/4">
                          <FormField
                            control={form.control}
                            name="pipeline_version_id"
                            render={({ field }) => (
                              <FormItem className="space-y-1">
                                <div className="flex items-center gap-2">
                                  <PipelineVersionSelect
                                    versions={canvasVersions}
                                    value={field.value ?? ''}
                                    onChange={field.onChange}
                                  />
                                  {savedVersionOutdated && (
                                    <RAGFlowTooltip
                                      tooltip={t('flow.version.outdated')}
                                    >
                                      <span className="text-destructive cursor-help font-bold">
                                        !
                                      </span>
                                    </RAGFlowTooltip>
                                  )}
                                </div>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </div>
                      </div>
                    </FormItem>
                  )}
                  {showOperatorTabs && (
                    <PipelineOperatorTabs
                      nodes={operatorNodes}
                      activeTab={activeTab}
                      onTabChange={setActiveTab}
                      onOperatorValuesChange={handleOperatorValuesChange}
                      operatorValues={parserConfigValues}
                      operatorFormErrors={
                        errors.parser_config as
                          | Record<string, FieldErrors | undefined>
                          | undefined
                      }
                      fixedFileFormats
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
                    form.formState.isSubmitting ||
                    (operatorNodesLoading && operatorNodes.length === 0)
                  }
                >
                  {t('knowledgeConfiguration.save')}
                </ButtonLoading>
              </div>
            </form>
          </Form>

          <div className="flex-1 p-5 overflow-auto">
            {parseType === ParseType.BuiltIn && builtinPipelineId && (
              <ChunkMethodLearnMore parserId={builtinPipelineId} />
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
