import { Form } from '@/components/ui/form';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs-underlined';
import { DocumentParserType } from '@/constants/knowledge';
import { zodResolver } from '@hookform/resolvers/zod';
import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { TopTitle } from '../dataset-title';
import { ChunkMethodForm } from './chunk-method-form';
import ChunkMethodLearnMore from './chunk-method-learn-more';
import { formSchema } from './form-schema';
import { GeneralForm } from './general-form';
import { useFetchKnowledgeConfigurationOnMount } from './hooks';

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
      permission: 'me',
      parser_config: {
        layout_recognize: DocumentType.DeepDOC,
        chunk_token_num: 512,
        delimiter: `\n`,
        auto_keywords: 0,
        auto_questions: 0,
        html4excel: false,
        topn_tags: 3,
        raptor: {
          use_raptor: false,
          max_token: 256,
          threshold: 0.1,
          max_cluster: 64,
          random_seed: 0,
        },
        graphrag: {
          use_graphrag: false,
          entity_types: initialEntityTypes,
          method: MethodValue.Light,
        },
      },
      pagerank: 0,
    },
  });

  useFetchKnowledgeConfigurationOnMount(form);

  const [currentTab, setCurrentTab] = useState<
    'generalForm' | 'chunkMethodForm'
  >('generalForm'); // currnet Tab state

  const parserId = useWatch({
    control: form.control,
    name: 'parser_id',
  });

  async function onSubmit(data: z.infer<typeof formSchema>) {
    console.log('🚀 ~ DatasetSettings ~ data:', data);
  }

  return (
    <section className="p-5 ">
      <TopTitle
        title={t('knowledgeDetails.configuration')}
        description={t('knowledgeConfiguration.titleDescription')}
      ></TopTitle>
      <div className="flex gap-14">
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6 basis-full min-w-[1000px] max-w-[1000px]"
          >
            <Tabs
              defaultValue="generalForm"
              onValueChange={(val) => {
                setCurrentTab(val);
              }}
            >
              <TabsList className="grid bg-background grid-cols-2 rounded-none bg-[#161618]">
                <TabsTrigger
                  value="generalForm"
                  className="group bg-transparent p-0 !border-transparent"
                >
                  <div className="flex w-full h-full justify-center	items-center	bg-[#161618]">
                    <span className="h-full group-data-[state=active]:border-b-2 border-white	">
                      General
                    </span>
                  </div>
                </TabsTrigger>
                <TabsTrigger
                  value="chunkMethodForm"
                  className="group bg-transparent p-0 !border-transparent"
                >
                  <div className="flex w-full h-full justify-center	items-center	bg-[#161618]">
                    <span className="h-full group-data-[state=active]:border-b-2 border-white	">
                      Chunk Method
                    </span>
                  </div>
                </TabsTrigger>
              </TabsList>
              <TabsContent value="generalForm">
                <GeneralForm></GeneralForm>
              </TabsContent>
              <TabsContent value="chunkMethodForm">
                <ChunkMethodForm></ChunkMethodForm>
              </TabsContent>
            </Tabs>
          </form>
        </Form>
        <ChunkMethodLearnMore tab={currentTab} parserId={parserId} />
      </div>
    </section>
  );
}
