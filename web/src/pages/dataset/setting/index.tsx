import { Form } from '@/components/ui/form';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { DocumentParserType } from '@/constants/knowledge';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import CategoryPanel from './category-panel';
import { ChunkMethodForm } from './chunk-method-form';
import { GeneralForm } from './general-form';

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
  const formSchema = z.object({
    name: z.string().min(1, {
      message: 'Username must be at least 2 characters.',
    }),
    description: z.string().min(2, {
      message: 'Username must be at least 2 characters.',
    }),
    avatar: z.instanceof(File),
    permission: z.string(),
    parser_id: z.string(),
    parser_config: z.object({
      layout_recognize: z.string(),
      chunk_token_num: z.number(),
      delimiter: z.string(),
      auto_keywords: z.number(),
      auto_questions: z.number(),
      html4excel: z.boolean(),
      tag_kb_ids: z.array(z.string()),
      topn_tags: z.number(),
      raptor: z.object({
        use_raptor: z.boolean(),
        prompt: z.string(),
        max_token: z.number(),
        threshold: z.number(),
        max_cluster: z.number(),
        random_seed: z.number(),
      }),
      graphrag: z.object({
        use_graphrag: z.boolean(),
        entity_types: z.array(z.string()),
        method: z.string(),
        resolution: z.boolean(),
        community: z.boolean(),
      }),
    }),
    pagerank: z.number(),
    // icon: z.array(z.instanceof(File)),
  });

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

  async function onSubmit(data: z.infer<typeof formSchema>) {
    console.log('🚀 ~ DatasetSettings ~ data:', data);
  }

  return (
    <section className="p-5 ">
      <div className="pb-5">
        <div className="text-2xl font-semibold">Configuration</div>
        <p className="text-text-sub-title pt-2">
          Update your knowledge base configuration here, particularly the chunk
          method.
        </p>
      </div>
      <div className="flex gap-14">
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6 basis-full"
          >
            <Tabs defaultValue="account">
              <TabsList className="grid w-full grid-cols-2">
                <TabsTrigger value="account">Account</TabsTrigger>
                <TabsTrigger value="password">Password</TabsTrigger>
              </TabsList>
              <TabsContent value="account">
                <GeneralForm></GeneralForm>
              </TabsContent>
              <TabsContent value="password">
                <ChunkMethodForm></ChunkMethodForm>
              </TabsContent>
            </Tabs>
          </form>
        </Form>
        <CategoryPanel chunkMethod={DocumentParserType.Naive}></CategoryPanel>
      </div>
    </section>
  );
}
