'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { ModelTreeSelectFormField } from '@/components/model-tree-select';
import { MemoriesFormField } from '@/components/memories-form-field';
import { Button, ButtonLoading } from '@/components/ui/button';
import { DialogFooter } from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { TagRenameId } from '@/constants/knowledge';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IModalProps } from '@/interfaces/common';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { FlowType } from './constant';
import { NameFormField, NameFormSchema } from './name-form-field';
import {
  ModelBindingCount,
  RetrievalBindings,
} from './template-retrieval-binding';

export type CreateAgentFormProps = IModalProps<any> & {
  loading?: boolean;
  selectedType: FlowType;
  // Templates may ship retrieval steps without any dataset/memory bound. When
  // set, the form asks the user to bind them before creating the agent, so
  // the canvas never ends up with a retrieval that fails at runtime.
  retrievalBindings?: RetrievalBindings;
  modelBindings?: ModelBindingCount;
};

export const FormSchema = z.object({
  ...NameFormSchema,
  tag: z.string().trim().optional(),
  description: z.string().trim().optional(),
  type: z.nativeEnum(FlowType),
  dataset_ids: z.array(z.string()).optional(),
  dataset_bindings: z.array(z.array(z.string())).optional(),
  memory_ids: z.array(z.string()).optional(),
  llm_id: z.string().optional(),
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export function CreateAgentForm({
  hideModal,
  onOk,
  loading,
  selectedType,
  retrievalBindings,
  modelBindings,
}: CreateAgentFormProps) {
  const { t } = useTranslation();
  const { navigateToCompilationEditor } = useNavigatePage();

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', type: selectedType },
  });

  // Compilation operators are configured on the edit-next page, so the dialog
  // skips the name field and turns the submit button into a navigation step.
  const isCompiler = selectedType === FlowType.Compiler;
  const modelRequired = (modelBindings?.modelCount ?? 0) > 0;

  const handleNext = useCallback(() => {
    navigateToCompilationEditor();
  }, [navigateToCompilationEditor]);

  async function onSubmit(data: FormSchemaType) {
    if (modelRequired && !data.llm_id) {
      form.setError('llm_id', {
        type: 'manual',
        message: t('common.pleaseSelect'),
      });
      return;
    }
    const datasetBlocks = retrievalBindings?.datasetBlocks ?? [];
    if (datasetBlocks.length === 1 && isEmpty(data.dataset_ids)) {
      form.setError('dataset_ids', {
        type: 'manual',
        message: t('flow.retrievalDatasetRequired'),
      });
      return;
    }
    if (datasetBlocks.length > 1) {
      const missingIndex = datasetBlocks.findIndex((_, index) =>
        isEmpty(data.dataset_bindings?.[index]),
      );
      if (missingIndex >= 0) {
        form.setError(`dataset_bindings.${missingIndex}`, {
          type: 'manual',
          message: t('flow.retrievalDatasetRequired'),
        });
        return;
      }
    }
    if ((retrievalBindings?.memoryCount ?? 0) > 0 && isEmpty(data.memory_ids)) {
      form.setError('memory_ids', {
        type: 'manual',
        message: t('flow.retrievalMemoryRequired'),
      });
      return;
    }
    const ret = await onOk?.(data);
    if (ret) {
      hideModal?.();
    }
  }

  const datasetBlocks = retrievalBindings?.datasetBlocks ?? [];
  const datasetHint =
    datasetBlocks.length === 1
      ? t('flow.retrievalTemplateDatasetHint', {
          num: 1,
        })
      : undefined;
  const memoryHint = retrievalBindings?.memoryCount
    ? t('flow.retrievalTemplateMemoryHint', {
        num: retrievalBindings.memoryCount,
      })
    : undefined;

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={TagRenameId}
      >
        {!isCompiler && <NameFormField></NameFormField>}
        {!isCompiler && modelRequired && (
          <ModelTreeSelectFormField
            name="llm_id"
            label={t('chat.model')}
            modelTypes={modelBindings?.modelTypes}
            required
          />
        )}
        {!isCompiler && datasetBlocks.length === 1 && (
          <section className="space-y-4">
            <p className="text-sm text-text-secondary">{datasetHint}</p>
            <KnowledgeBaseFormField required showVariable={false} />
          </section>
        )}
        {!isCompiler && datasetBlocks.length > 1 && (
          <section className="space-y-4">
            {datasetBlocks.map((block, index) => (
              <KnowledgeBaseFormField
                key={block.blockId}
                required
                showVariable={false}
                name={`dataset_bindings.${index}`}
                label={block.displayName}
              />
            ))}
          </section>
        )}
        {!isCompiler && memoryHint && (
          <section className="space-y-4">
            <p className="text-sm text-text-secondary">{memoryHint}</p>
            <MemoriesFormField label={t('header.memories')} required />
          </section>
        )}
      </form>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={hideModal}>
          {t('common.cancel')}
        </Button>
        {isCompiler ? (
          <Button type="button" data-testid="agent-next" onClick={handleNext}>
            {t('common.next')}
          </Button>
        ) : (
          <ButtonLoading
            data-testid="agent-save"
            type="submit"
            form={TagRenameId}
            loading={loading}
          >
            {t('common.confirm')}
          </ButtonLoading>
        )}
      </DialogFooter>
    </Form>
  );
}
