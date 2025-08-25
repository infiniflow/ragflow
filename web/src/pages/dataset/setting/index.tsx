import { Form } from '@/components/ui/form';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs-underlined';
import { DocumentParserType } from '@/constants/knowledge';
import { PermissionRole } from '@/constants/permission';
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
          use_raptor: false,
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
            <Tabs
              defaultValue="generalForm"
              onValueChange={(val) => {
                setCurrentTab(val);
              }}
              className="h-full flex flex-col"
            >
              <TabsList className="grid bg-transparent grid-cols-2 rounded-none text-foreground">
                <TabsTrigger
                  value="generalForm"
                  className="group bg-transparent p-0 !border-transparent"
                >
                  <div className="flex w-full h-full justify-center	items-center">
                    <span className="h-full group-data-[state=active]:border-b-2 border-foreground	">
                      {t('knowledgeDetails.general')}
                    </span>
                  </div>
                </TabsTrigger>
                <TabsTrigger
                  value="chunkMethodForm"
                  className="group bg-transparent p-0 !border-transparent"
                >
                  <div className="flex w-full h-full justify-center	items-center">
                    <span className="h-full group-data-[state=active]:border-b-2 border-foreground	">
                      {t('knowledgeDetails.chunkMethodTab')}
                    </span>
                  </div>
                </TabsTrigger>
              </TabsList>
              <TabsContent value="generalForm" className="flex-1 min-h-0">
                <GeneralForm></GeneralForm>
              </TabsContent>
              <TabsContent value="chunkMethodForm" className="flex-1 min-h-0">
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
