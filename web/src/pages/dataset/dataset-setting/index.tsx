import { DataFlowSelect } from '@/components/data-pipeline-select';
import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import { Button } from '@/components/ui/button';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { FormLayout } from '@/constants/form';
import { DocumentParserType } from '@/constants/knowledge';
import { PermissionRole } from '@/constants/permission';
import { IConnector, IKnowledge } from '@/interfaces/database/knowledge';
import { useDataSourceInfo } from '@/pages/user-setting/data-source/constant';
import { IDataSourceBase } from '@/pages/user-setting/data-source/interface';
import { zodResolver } from '@hookform/resolvers/zod';
import { createContext, useEffect, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { TopTitle } from '../dataset-title';
import {
  GenerateType,
  IGenerateLogButtonProps,
} from '../dataset/generate-button/generate';
import { ChunkMethodForm } from './chunk-method-form';
import ChunkMethodLearnMore from './chunk-method-learn-more';
import LinkDataSource, {
  IDataSourceNodeProps,
} from './components/link-data-source';
import { MainContainer } from './configuration-form-container';
import { ChunkMethodItem, ParseTypeItem } from './configuration/common-item';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import { useFetchKnowledgeConfigurationOnMount } from './hooks';
import { SavingButton } from './saving-button';
const enum DocumentType {
  DeepDOC = 'DeepDOC',
  PlainText = 'Plain Text',
}
export const DataSetContext = createContext<{
  loading: boolean;
  knowledgeDetails: IKnowledge;
}>({ loading: false, knowledgeDetails: {} as IKnowledge });

const initialEntityTypes = [
  'organization',
  'person',
  'geo',
  'event',
  'category',
];

const enum MethodValue {
  General = 'general',
  Light = 'light',
}

export default function DatasetSettings() {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      name: '',
      parser_id: DocumentParserType.Naive,
      permission: PermissionRole.Me,
      language: 'English',
      parser_config: {
        layout_recognize: DocumentType.DeepDOC,
        chunk_token_num: 512,
        delimiter: `\n`,
        enable_children: false,
        children_delimiter: `\n`,
        auto_keywords: 0,
        auto_questions: 0,
        html4excel: false,
        topn_tags: 3,
        toc_extraction: false,
        image_table_context_window: 0,
        overlapped_percent: 0,
        // MinerU-specific defaults
        mineru_parse_method: 'auto',
        mineru_formula_enable: true,
        mineru_table_enable: true,
        mineru_lang: 'English',
        raptor: {
          use_raptor: true,
          max_token: 256,
          threshold: 0.1,
          max_cluster: 64,
          random_seed: 0,
          scope: 'file',
          prompt: t('knowledgeConfiguration.promptText'),
        },
        graphrag: {
          use_graphrag: true,
          entity_types: initialEntityTypes,
          method: MethodValue.Light,
        },
        metadata: {
          type: 'object',
          properties: {},
          additionalProperties: false,
        },
        built_in_metadata: [],
        enable_metadata: false,
        llm_id: '',
      },
      pipeline_id: '',
      parseType: 1,
      pagerank: 0,
      connectors: [],
    },
  });
  const { dataSourceInfo } = useDataSourceInfo();
  const { knowledgeDetails, loading: datasetSettingLoading } =
    useFetchKnowledgeConfigurationOnMount(form);
  // const [pipelineData, setPipelineData] = useState<IDataPipelineNodeProps>();
  const [sourceData, setSourceData] = useState<IDataSourceNodeProps[]>();
  const [graphRagGenerateData, setGraphRagGenerateData] =
    useState<IGenerateLogButtonProps>();
  const [raptorGenerateData, setRaptorGenerateData] =
    useState<IGenerateLogButtonProps>();

  useEffect(() => {
    console.log('🚀 ~ DatasetSettings ~ knowledgeDetails:', knowledgeDetails);
    if (knowledgeDetails) {
      // const data: IDataPipelineNodeProps = {
      //   id: knowledgeDetails.pipeline_id,
      //   name: knowledgeDetails.pipeline_name,
      //   avatar: knowledgeDetails.pipeline_avatar,
      //   linked: true,
      // };
      // setPipelineData(data);

      const source_data: IDataSourceNodeProps[] =
        knowledgeDetails?.connectors?.map((connector) => {
          return {
            ...connector,
            icon:
              dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
                ?.icon || '',
          };
        });

      setSourceData(source_data);

      setGraphRagGenerateData({
        finish_at: knowledgeDetails.graphrag_task_finish_at,
        task_id: knowledgeDetails.graphrag_task_id,
      } as IGenerateLogButtonProps);
      setRaptorGenerateData({
        finish_at: knowledgeDetails.raptor_task_finish_at,
        task_id: knowledgeDetails.raptor_task_id,
      } as IGenerateLogButtonProps);
      form.setValue('parseType', knowledgeDetails.pipeline_id ? 2 : 1);
      form.setValue('pipeline_id', knowledgeDetails.pipeline_id || '');
    }
  }, [knowledgeDetails, form]);

  async function onSubmit(data: z.infer<typeof formSchema>) {
    try {
      console.log('Form validation passed, submit data', data);
    } catch (error) {
      console.error('An error occurred during submission:', error);
    }
  }
  // const handleLinkOrEditSubmit = (
  //   data: IDataPipelineSelectNode | undefined,
  // ) => {
  //   console.log('🚀 ~ DatasetSettings ~ data:', data);
  //   if (data) {
  //     setPipelineData(data);
  //     form.setValue('pipeline_id', data.id || '');
  //     // form.setValue('pipeline_name', data.name || '');
  //     // form.setValue('pipeline_avatar', data.avatar || '');
  //   }
  // };

  const handleLinkOrEditSubmit = (data: IConnector[] | undefined) => {
    if (data) {
      const connectors = data.map((connector) => {
        return {
          ...connector,
          auto_parse: connector.auto_parse === '0' ? '0' : '1',
          icon:
            dataSourceInfo[connector.source as keyof typeof dataSourceInfo]
              ?.icon || '',
        };
      });
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
      // form.setValue('pipeline_name', data.name || '');
      // form.setValue('pipeline_avatar', data.avatar || '');
    }
  };

  const handleDeletePipelineTask = (type: GenerateType) => {
    if (type === GenerateType.KnowledgeGraph) {
      setGraphRagGenerateData({
        finish_at: '',
        task_id: '',
      } as IGenerateLogButtonProps);
    } else if (type === GenerateType.Raptor) {
      setRaptorGenerateData({
        finish_at: '',
        task_id: '',
      } as IGenerateLogButtonProps);
    }
  };

  const parseType = useWatch({
    control: form.control,
    name: 'parseType',
    defaultValue: knowledgeDetails.pipeline_id ? 2 : 1,
  });
  const selectedTag = useWatch({
    name: 'parser_id',
    control: form.control,
  });
  useEffect(() => {
    if (parseType === 1) {
      form.setValue('pipeline_id', '');
    }
    console.log('parseType', parseType);
  }, [parseType, form]);

  const unbindFunc = (data: IDataSourceBase) => {
    if (data) {
      const connectors = sourceData?.filter((connector) => {
        return connector.id !== data.id;
      });
      console.log('🚀 ~ DatasetSettings ~ connectors:', connectors);
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
      // form.setValue('pipeline_name', data.name || '');
      // form.setValue('pipeline_avatar', data.avatar || '');
    }
  };
  const handleAutoParse = ({
    source_id,
    isAutoParse,
  }: {
    source_id: string;
    isAutoParse: boolean;
  }) => {
    if (source_id) {
      const connectors = sourceData?.map((connector) => {
        if (connector.id === source_id) {
          return {
            ...connector,
            auto_parse: isAutoParse ? '1' : '0',
          };
        }
        return connector;
      });
      setSourceData(connectors as IDataSourceNodeProps[]);
      form.setValue('connectors', connectors || []);
    }
  };

  return (
    <section className="p-5 h-full flex flex-col">
      <TopTitle
        title={t('knowledgeDetails.configuration')}
        description={t('knowledgeConfiguration.titleDescription')}
      ></TopTitle>
      <div className="flex gap-14 flex-1 min-h-0">
        <DataSetContext.Provider
          value={{
            loading: datasetSettingLoading,
            knowledgeDetails: knowledgeDetails,
          }}
        >
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6 ">
              <div className="w-[768px] h-[calc(100vh-240px)] pr-1 overflow-y-auto scrollbar-auto">
                <MainContainer className="text-text-secondary">
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.baseInfo')}
                  </div>
                  <GeneralForm></GeneralForm>

                  <Divider />
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.dataPipeline')}
                  </div>
                  <ParseTypeItem line={1} />
                  {parseType === 1 && (
                    <ChunkMethodItem line={1}></ChunkMethodItem>
                  )}
                  {parseType === 2 && (
                    <DataFlowSelect
                      isMult={false}
                      showToDataPipeline={true}
                      formFieldName="pipeline_id"
                      layout={FormLayout.Horizontal}
                    />
                  )}

                  {/* <Divider /> */}
                  {parseType === 1 && <ChunkMethodForm />}

                  {/* <LinkDataPipeline
                  data={pipelineData}
                  handleLinkOrEditSubmit={handleLinkOrEditSubmit}
                /> */}
                  <Divider />
                  <LinkDataSource
                    data={sourceData}
                    handleLinkOrEditSubmit={handleLinkOrEditSubmit}
                    unbindFunc={unbindFunc}
                    handleAutoParse={handleAutoParse}
                  />
                  <Divider />
                  <div className="text-base font-medium text-text-primary">
                    {t('knowledgeConfiguration.globalIndex')}
                  </div>
                  <GraphRagItems
                    className="border-none p-0"
                    data={graphRagGenerateData as IGenerateLogButtonProps}
                    onDelete={() =>
                      handleDeletePipelineTask(GenerateType.KnowledgeGraph)
                    }
                  ></GraphRagItems>
                  <Divider />
                  <RaptorFormFields
                    data={raptorGenerateData as IGenerateLogButtonProps}
                    onDelete={() =>
                      handleDeletePipelineTask(GenerateType.Raptor)
                    }
                  ></RaptorFormFields>
                </MainContainer>
              </div>
              <div className="text-right items-center flex justify-end gap-3 w-[768px]">
                <Button
                  type="reset"
                  className="bg-transparent text-color-white hover:bg-transparent border-gray-500 border-[1px]"
                  onClick={() => {
                    form.reset();
                  }}
                >
                  {t('knowledgeConfiguration.cancel')}
                </Button>
                <SavingButton></SavingButton>
              </div>
            </form>
          </Form>
          <div className="flex-1">
            {parseType === 1 && <ChunkMethodLearnMore parserId={selectedTag} />}
          </div>
        </DataSetContext.Provider>
      </div>
    </section>
  );
}
