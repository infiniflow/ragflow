'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Card, CardContent } from '@/components/ui/card';
import { Form } from '@/components/ui/form';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { BrainCircuit, Check, Route } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { FlowType } from './constant';
import { NameFormField, NameFormSchema } from './name-form-field';

export type CreateAgentFormProps = IModalProps<any> & {
  shouldChooseAgent?: boolean;
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
      {Object.values(FlowType).map((val) => {
        const isActive = value === val;
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
                {val === FlowType.Agent ? (
                  <BrainCircuit className="size-6" />
                ) : (
                  <Route className="size-6" />
                )}
                <p>
                  {t(
                    `flow.${val === FlowType.Agent ? 'createAgent' : 'createPipeline'}`,
                  )}
                </p>
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
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export function CreateAgentForm({
  hideModal,
  onOk,
  shouldChooseAgent = false,
}: CreateAgentFormProps) {
  const { t } = useTranslation();

  const form = useForm<FormSchemaType>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', type: FlowType.Agent },
  });

  async function onSubmit(data: FormSchemaType) {
    const ret = await onOk?.(data);
    if (ret) {
      hideModal?.();
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={TagRenameId}
      >
        {shouldChooseAgent && (
          <RAGFlowFormItem
            required
            name="type"
            label={t('flow.chooseAgentType')}
          >
            <FlowTypeCards></FlowTypeCards>
          </RAGFlowFormItem>
        )}
        <NameFormField></NameFormField>
      </form>
    </Form>
  );
}
