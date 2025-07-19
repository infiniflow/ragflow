import { EntityTypesFormField } from '@/components/entity-types-form-field';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { useFetchKnowledgeBaseConfiguration } from '@/hooks/knowledge-hooks';
import { useUpdateKnowledge, useKnowledgeBaseId } from '@/hooks/use-knowledge-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { upperFirst } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

const enum MethodValue {
  General = 'general',
  Light = 'light',
}

const GraphRagConfiguration: React.FC = () => {
  const { t } = useTranslation();
  const knowledgeBaseId = useKnowledgeBaseId();
  const { data: knowledgeDetails } = useFetchKnowledgeBaseConfiguration();
  const { saveKnowledgeConfiguration, loading } = useUpdateKnowledge();
  
  // Use knowledge details ID if URL param is not available
  const actualKbId = knowledgeBaseId || knowledgeDetails?.id;

  const methodOptions = useMemo(() => {
    return [MethodValue.Light, MethodValue.General].map((x) => ({
      value: x,
      label: upperFirst(x),
    }));
  }, []);

  const FormSchema = z.object({
    graphrag: z.object({
      entity_types: z.array(z.string()),
      method: z.enum([MethodValue.Light, MethodValue.General]),
    }),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      graphrag: {
        entity_types: ['organization', 'person', 'geo', 'event', 'category'],
        method: MethodValue.Light,
      },
    },
  });

  const renderWideTooltip = useCallback(
    (title: React.ReactNode | string) => {
      return typeof title === 'string' ? t(title) : title;
    },
    [t],
  );

  const onSubmit = async (data: z.infer<typeof FormSchema>) => {
    try {
      await saveKnowledgeConfiguration({
        kb_id: actualKbId,
        name: knowledgeDetails.name,
        description: knowledgeDetails.description,
        parser_id: knowledgeDetails.parser_id,
        parser_config: {
          ...knowledgeDetails.parser_config,
          graphrag: {
            // Preserve existing use_graphrag value, only update entity_types and method
            use_graphrag: knowledgeDetails.parser_config?.graphrag?.use_graphrag || false,
            ...data.graphrag,
          },
        },
      });
    } catch (error) {
      console.error('Failed to update GraphRAG configuration:', error);
    }
  };

  // Update form when knowledge details change
  useEffect(() => {
    if (knowledgeDetails?.parser_config?.graphrag) {
      const graphragConfig = knowledgeDetails.parser_config.graphrag;
      form.reset({
        graphrag: {
          entity_types: graphragConfig.entity_types || ['organization', 'person', 'geo', 'event', 'category'],
          method: graphragConfig.method || MethodValue.Light,
        },
      });
    }
  }, [knowledgeDetails, form]);

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
        <div className="rounded-lg border p-3 bg-blue-50/50 border-blue-200">
          <div className="text-sm text-gray-700">
            Construct a knowledge graph over file chunks of the current knowledge base to enhance multi-hop question-answering involving nested logic. See{' '}
            <a 
              href="https://ragflow.io/docs/dev/construct_knowledge_graph" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-blue-600 hover:text-blue-800 underline"
            >
              https://ragflow.io/docs/dev/construct_knowledge_graph
            </a>{' '}
            for details.
          </div>
        </div>

        <EntityTypesFormField name="graphrag.entity_types" />

        <FormField
          control={form.control}
          name="graphrag.method"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('knowledgeConfiguration.graphRagMethod', 'GraphRAG Method')}
              </FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={methodOptions}
                />
              </FormControl>
              <div className="text-xs text-muted-foreground">
                <div
                  dangerouslySetInnerHTML={{
                    __html: t('knowledgeConfiguration.graphRagMethodTip', 'Choose between Light and General methods for entity extraction'),
                  }}
                />
              </div>
              <FormMessage />
            </FormItem>
          )}
        />

        <Button type="submit" className="w-full" size="sm" disabled={loading}>
          {loading ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
        </Button>
      </form>
    </Form>
  );
};

export default GraphRagConfiguration;