import { IDataPipelineSelectNode } from '@/components/data-pipeline-select';
import GraphRagItems from '@/components/parse-configuration/graph-rag-form-fields';
import RaptorFormFields from '@/components/parse-configuration/raptor-form-fields';
import { Button } from '@/components/ui/button';
import Divider from '@/components/ui/divider';
import { Form } from '@/components/ui/form';
import { DocumentParserType } from '@/constants/knowledge';
import { PermissionRole } from '@/constants/permission';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { TopTitle } from '../dataset-title';
import { IGenerateLogButtonProps } from '../dataset/generate-button/generate';
import LinkDataPipeline, {
  IDataPipelineNodeProps,
} from './components/link-data-pipeline';
import { MainContainer } from './configuration-form-container';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import { useFetchKnowledgeConfigurationOnMount } from './hooks';
import { SavingButton } from './saving-button';
const enum DocumentType {
  DeepDOC = 'DeepDOC',
  PlainText = 'Plain Text',
}

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
      parser_config: {
        layout_recognize: DocumentType.DeepDOC,
        chunk_token_num: 512,
        delimiter: `\n`,
        auto_keywords: 0,
        auto_questions: 0,
        html4excel: false,
        topn_tags: 3,
        raptor: {
          use_raptor: true,
          max_token: 256,
          threshold: 0.1,
          max_cluster: 64,
          random_seed: 0,
          prompt: t('knowledgeConfiguration.promptText'),
        },
        graphrag: {
          use_graphrag: true,
          entity_types: initialEntityTypes,
          method: MethodValue.Light,
        },
      },
      pipeline_id: '',
      pagerank: 0,
    },
  });

  const knowledgeDetails = useFetchKnowledgeConfigurationOnMount(form);

  const [pipelineData, setPipelineData] = useState<IDataPipelineNodeProps>();
  const [graphRagGenerateData, setGraphRagGenerateData] =
    useState<IGenerateLogButtonProps>();
  const [raptorGenerateData, setRaptorGenerateData] =
    useState<IGenerateLogButtonProps>();
  useEffect(() => {
    console.log('🚀 ~ DatasetSettings ~ knowledgeDetails:', knowledgeDetails);
    if (knowledgeDetails) {
      const data: IDataPipelineNodeProps = {
        id: knowledgeDetails.pipeline_id,
        name: knowledgeDetails.pipeline_name,
        avatar: knowledgeDetails.pipeline_avatar,
        linked: true,
      };
      setPipelineData(data);
      setGraphRagGenerateData({
        finish_at: knowledgeDetails.mindmap_task_finish_at,
        task_id: knowledgeDetails.mindmap_task_id,
      } as IGenerateLogButtonProps);
      setRaptorGenerateData({
        finish_at: knowledgeDetails.raptor_task_finish_at,
        task_id: knowledgeDetails.raptor_task_id,
      } as IGenerateLogButtonProps);
    }
  }, [knowledgeDetails]);

  async function onSubmit(data: z.infer<typeof formSchema>) {
    try {
      console.log('Form validation passed, submit data', data);
    } catch (error) {
      console.error('An error occurred during submission:', error);
    }
  }
  const handleLinkOrEditSubmit = (
    data: IDataPipelineSelectNode | undefined,
  ) => {
    console.log('🚀 ~ DatasetSettings ~ data:', data);
    if (data) {
      setPipelineData(data);
      form.setValue('pipeline_id', data.id || '');
      // form.setValue('pipeline_name', data.name || '');
      // form.setValue('pipeline_avatar', data.avatar || '');
    }
  };
  return (
    <section className="p-5 h-full flex flex-col">
      <TopTitle
        title={t('knowledgeDetails.configuration')}
        description={t('knowledgeConfiguration.titleDescription')}
      ></TopTitle>
      <div className="flex gap-14 flex-1 min-h-0">
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6 flex-1"
          >
            <div className="w-[768px] h-[calc(100vh-240px)] pr-1 overflow-y-auto scrollbar-auto">
              <MainContainer className="text-text-secondary">
                <GeneralForm></GeneralForm>
                <Divider />

                <GraphRagItems
                  className="border-none p-0"
                  data={graphRagGenerateData as IGenerateLogButtonProps}
                ></GraphRagItems>
                <Divider />
                <RaptorFormFields
                  data={raptorGenerateData as IGenerateLogButtonProps}
                ></RaptorFormFields>
                <Divider />
                <LinkDataPipeline
                  data={pipelineData}
                  handleLinkOrEditSubmit={handleLinkOrEditSubmit}
                />
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
      </div>
    </section>
  );
}
