import { ParserTreeSelect } from '@/components/parser-tree-select';
import { useParserTreeData } from '@/components/parser-tree-select/use-parser-tree-data';
import { usePrefillLatestPipelineVersion } from '@/components/parser-tree-select/use-prefill-latest-pipeline-version';
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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldErrors, useForm, useFormState, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import ChunkMethodLearnMore from './chunk-method-learn-more';
import LinkDataSource, {
  IDataSourceNodeProps,
} from './components/link-data-source';
import { buildFormSchema, DatasetSettingFormValues } from './form-schema';
import { GeneralForm } from './general-form';
import {
  useConnectorHandlers,
  useFetchDatasetSettingOnMount,
  usePipelineDataList,
  useSaveDatasetSetting,
} from './hooks';
import { normalizeParserConfigForComparison } from './parser-config';
import { useFetchCanvasVersionList } from '@/hooks/use-agent-request';
import { useRevealSubmitErrors } from './use-reveal-submit-errors';
import { ApplyParserConfigDialog } from './apply-parser-config-dialog';

export default function DatasetSetting() {
  const { t } = useTranslation();

  // The missing verdict lives behind a ref because the schema must exist
  // before useForm runs, while the verdict derives from the form's watched
  // values — see buildFormSchema.
  const parserOptionMissingRef = useRef(false);
  const formSchema = useMemo(
    () => buildFormSchema(() => parserOptionMissingRef.current),
    [],
  );

  const form = useForm<DatasetSettingFormValues>({
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
  const [applyParserConfigOpen, setApplyParserConfigOpen] = useState(false);

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

  const parserSelectValue = buildParserSelectValue({
    parseType,
    parserId: builtinPipelineId,
    pipelineId,
    pipelineVersionId,
  });

  // The merged parser tree owns the option data; reusing its existence
  // verdict (same React Query cache, no extra fetches) flags a saved parser
  // or pinned pipeline version that has since been deleted.
  const { selectedLeafExists, loading: parserOptionsLoading } =
    useParserTreeData({ value: parserSelectValue });
  const parserOptionMissing =
    !!parserSelectValue && !parserOptionsLoading && !selectedLeafExists;

  // Keep the schema's missing verdict current; declared before the
  // revalidation effect below so trigger() reads the fresh value.
  useEffect(() => {
    parserOptionMissingRef.current = parserOptionMissing;
  }, [parserOptionMissing]);

  // A persisted value never fires onChange validation, so once the parser
  // options have loaded, revalidate explicitly — the saved parser or pinned
  // version may have been deleted, and the error should be visible before
  // submit. Re-running on selection changes clears the error as soon as a
  // valid option is picked. (Same pattern as ModelTreeSelectFormField.)
  const trigger = form.trigger;
  const parserFieldName =
    parseType === ParseType.Pipeline ? 'pipeline_id' : 'parser_id';
  useEffect(() => {
    if (parserOptionsLoading || !parserSelectValue) return;
    trigger(parserFieldName);
  }, [trigger, parserOptionsLoading, parserSelectValue, parserFieldName]);

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

  // Published versions of the selected dataflow canvas, fetched here only to
  // prefill the latest version for unpinned datasets (design C1/C2); the
  // select itself lazy-loads whatever branches the user expands.
  const { data: canvasVersions } = useFetchCanvasVersionList(
    parseType === ParseType.Pipeline && pipelineId ? pipelineId : undefined,
  );
  // Show the outdated bang only while the form still holds the SAVED pin:
  // once the user picks a different (unsaved) version the bang would be
  // misleading (design D3: hint only, never auto-upgrade).
  const savedVersionOutdated =
    !!knowledgeDetails?.pipeline_version_outdated &&
    (pipelineVersionId ?? '') === (knowledgeDetails?.pipeline_version_id ?? '');

  // Design section 8 (T37): an unpinned dataset still pre-selects the latest
  // concrete canvas version once the version list resolves.
  usePrefillLatestPipelineVersion(
    form,
    knowledgeDetails?.pipeline_version_id,
    canvasVersions?.[0]?.id,
  );

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

  const { scrollContainerRef, handleInvalidSubmit } =
    useRevealSubmitErrors(setActiveTab);

  const handleSubmit = useCallback(
    async (data: DatasetSettingFormValues) => {
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
        (data.pipeline_version_id || '') !==
          (savedValues?.pipeline_version_id || '') ||
        !isEqual(parserConfig, savedParserConfig);

      const result = await handleSave(data);
      if (result.code === 0) {
        form.reset(data);
        if (knowledgeDetails.document_count > 0 && parserSettingsChanged) {
          setApplyParserConfigOpen(true);
        }
      }
    },
    [form, handleSave, knowledgeDetails.document_count, operatorNodes, t],
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
      <ApplyParserConfigDialog
        datasetId={knowledgeDetails.id}
        open={applyParserConfigOpen}
        onOpenChange={setApplyParserConfigOpen}
      />
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
                          render={({ fieldState }) => (
                            <FormItem className="space-y-1">
                              <ParserTreeSelect
                                value={parserSelectValue}
                                onChange={handleParserSelect}
                                outdated={savedVersionOutdated}
                                invalid={fieldState.invalid}
                              />
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </div>
                    </div>
                  </FormItem>
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
