'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import { KnowledgeBaseFormField } from '@/components/knowledge-base-item';
import { MemoriesFormField } from '@/components/memories-form-field';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button, ButtonLoading } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { DialogFooter } from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { Check } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { FlowType, FlowTypeConfig } from './constant';
import { NameFormField, NameFormSchema } from './name-form-field';
import { RetrievalBindingCount } from './template-retrieval-binding';

export type CreateAgentFormProps = IModalProps<any> & {
  loading?: boolean;
  showTypeCards?: boolean;
  // Templates may ship retrieval steps without any dataset/memory bound. When
  // set, the form asks the user to bind them before creating the agent, so
  // the canvas never ends up with a retrieval that fails at runtime.
  retrievalBindings?: RetrievalBindingCount;
};

type FlowTypeCardProps = {
  value?: FlowType;
  onChange?: (value: FlowType) => void;
};

function FlowTypeCards({ value, onChange }: FlowTypeCardProps) {
  const { t } = useTranslation();
  const handleChange = useCallback(
    (value: FlowType) => () => {
      onChange?.(value);
    },
    [onChange],
  );

  return (
    <section className="flex gap-10">
      {[FlowType.Agent, FlowType.Flow, FlowType.Compiler].map((val) => {
        const isActive = value === val;
        const config = FlowTypeConfig[val];
        const Icon = config.icon;
        return (
          <Card
            key={val}
            className={cn('flex-1 rounded-lg  border bg-transparent', {
              'border-text-primary': isActive,
              'border-border-default': !isActive,
            })}
          >
            <CardContent
              onClick={handleChange(val)}
              className={cn(
                'cursor-pointer p-5 text-text-secondary flex justify-between items-center',
                {
                  'text-text-primary': isActive,
                },
              )}
            >
              <div className="flex gap-2">
                <Icon className="size-6" />
                <p>{t(`flow.${config.labelKey}`)}</p>
              </div>
              {isActive && <Check />}
            </CardContent>
          </Card>
        );
      })}
    </section>
  );
}

export const FormSchema = z.object({
  ...NameFormSchema,
  tag: z.string().trim().optional(),
  description: z.string().trim().optional(),
  type: z.nativeEnum(FlowType).optional(),
  dataset_ids: z.array(z.string()).optional(),
  memory_ids: z.array(z.string()).optional(),
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export function CreateAgentForm({
  hideModal,
  onOk,
  loading,
  showTypeCards = false,
  retrievalBindings,
}: CreateAgentFormProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', type: FlowType.Agent },
  });

  const selectedType = useWatch({ control: form.control, name: 'type' });
  // Compilation operators are configured on the edit-next page, so the dialog
  // skips the name field and turns the submit button into a navigation step.
  const isCompiler = showTypeCards && selectedType === FlowType.Compiler;

  const handleNext = useCallback(() => {
    navigate(`${Routes.CompilationTemplatesEditNext}?source=agents`);
  }, [navigate]);

  async function onSubmit(data: FormSchemaType) {
    if (
      (retrievalBindings?.datasetCount ?? 0) > 0 &&
      isEmpty(data.dataset_ids)
    ) {
      form.setError('dataset_ids', {
        type: 'manual',
        message: t('flow.retrievalDatasetRequired'),
      });
      return;
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

  const datasetHint = retrievalBindings?.datasetCount
    ? t('flow.retrievalTemplateDatasetHint', {
        count: retrievalBindings.datasetCount,
      })
    : undefined;
  const memoryHint = retrievalBindings?.memoryCount
    ? t('flow.retrievalTemplateMemoryHint', {
        count: retrievalBindings.memoryCount,
      })
    : undefined;

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={TagRenameId}
      >
        {showTypeCards && (
          <RAGFlowFormItem
            required
            name="type"
            label={t('flow.chooseAgentType')}
          >
            <FlowTypeCards></FlowTypeCards>
          </RAGFlowFormItem>
        )}
        {!isCompiler && <NameFormField></NameFormField>}
        {!isCompiler && datasetHint && (
          <section className="space-y-4">
            <p className="text-sm text-text-secondary">{datasetHint}</p>
            <KnowledgeBaseFormField required showVariable={false} />
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
