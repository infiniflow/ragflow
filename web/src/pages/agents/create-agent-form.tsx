'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

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

export type CreateAgentFormProps = IModalProps<any> & {
  loading?: boolean;
  showTypeCards?: boolean;
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
      {[FlowType.Flow, FlowType.Compiler, FlowType.Agent].map((val) => {
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
});

export type FormSchemaType = z.infer<typeof FormSchema>;

export function CreateAgentForm({
  hideModal,
  onOk,
  loading,
  showTypeCards = false,
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
