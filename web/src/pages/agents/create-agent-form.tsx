'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Card, CardContent } from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { TagRenameId } from '@/pages/add-knowledge/constant';
import { BrainCircuit, Check, Route } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

export type CreateAgentFormProps = IModalProps<any> & {
  shouldChooseAgent?: boolean;
};

enum FlowType {
  Agent = 'agent',
  Flow = 'flow',
}

type FlowTypeCardProps = {
  value?: FlowType;
  onChange?: (value: FlowType) => void;
};
function FlowTypeCards({ value, onChange }: FlowTypeCardProps) {
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
              'border-bg-base': isActive,
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
                <p>{val}</p>
              </div>
              {isActive && <Check />}
            </CardContent>
          </Card>
        );
      })}
    </section>
  );
}

export function CreateAgentForm({
  hideModal,
  onOk,
  shouldChooseAgent = false,
}: CreateAgentFormProps) {
  const { t } = useTranslation();
  const FormSchema = z.object({
    name: z
      .string()
      .min(1, {
        message: t('common.namePlaceholder'),
      })
      .trim(),
    tag: z.string().trim().optional(),
    description: z.string().trim().optional(),
    type: z.nativeEnum(FlowType).optional(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', type: FlowType.Agent },
  });

  async function onSubmit(data: z.infer<typeof FormSchema>) {
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
          <RAGFlowFormItem required name="type" label={t('common.type')}>
            <FlowTypeCards></FlowTypeCards>
          </RAGFlowFormItem>
        )}
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('common.name')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('common.namePlaceholder')}
                  {...field}
                  autoComplete="off"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}
