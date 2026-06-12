import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { CompilationTemplate } from '@/interfaces/database/compilation-template';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ArtifactExtras } from './components/artifact-extras';
import { BuiltinTemplatePopover } from './components/builtin-template-popover';
import { EntityRelationSection } from './components/entity-relation-section';
import { GlobalRulesBlock } from './components/global-rules-block';
import { useTemplateFormState } from './hooks/use-template-form-state';
import {
  buildFieldTemplateMaps,
  compilationTemplateFormSchema,
  CompilationTemplateFormValues,
  emptyFormValues,
  formValuesToTemplateConfig,
  templateConfigToFormValues,
  TEXT_FIELD_MAX,
} from './interface';

interface EditTemplateFormProps {
  initial?: CompilationTemplate;
  savedTemplates?: CompilationTemplate[];
  onSubmit: (values: {
    name: string;
    description?: string;
    kind: CompilationTemplateFormValues['kind'];
    config: ReturnType<typeof formValuesToTemplateConfig>;
  }) => Promise<void> | void;
  onCancel: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  loading?: boolean;
}

/**
 * Editor body. Owned by EditTemplateDialog so it can render inside a
 * Dialog or any other shell. Handles its own form state, dirty tracking
 * (delegated to {@link useTemplateFormState}) and built-in seeding.
 */
export function EditTemplateForm({
  initial,
  savedTemplates = [],
  onSubmit,
  onCancel,
  onDirtyChange,
  loading,
}: EditTemplateFormProps) {
  const { t } = useTranslation();
  const { data: builtins } = useFetchBuiltinCompilationTemplates();

  const defaultValues = useMemo<CompilationTemplateFormValues>(() => {
    if (initial) {
      return templateConfigToFormValues(
        initial.name,
        initial.description,
        initial.config,
      );
    }
    return emptyFormValues();
  }, [initial]);

  const form = useForm<CompilationTemplateFormValues>({
    resolver: zodResolver(compilationTemplateFormSchema),
    defaultValues,
  });

  // Re-seed when switching between create/edit without unmounting.
  useEffect(() => {
    form.reset(defaultValues);
  }, [defaultValues, form]);

  useEffect(() => {
    onDirtyChange?.(form.formState.isDirty);
  }, [form.formState.isDirty, onDirtyChange]);

  const {
    pendingBuiltin,
    handleSelectBuiltin,
    confirmApplyPendingBuiltin,
    cancelPendingBuiltin,
  } = useTemplateFormState(
    form,
    initial?.name ?? '',
    initial?.description ?? '',
  );

  const kind = form.watch('kind');
  const fieldTemplates = useMemo(
    () => buildFieldTemplateMaps(builtins.map((builtin) => builtin.config)),
    [builtins],
  );

  const handleSubmit = form.handleSubmit(async (values) => {
    const normalizedName = values.name.trim();
    const hasDuplicatedName = savedTemplates.some(
      (template) =>
        template.id !== initial?.id &&
        template.name.trim().toLowerCase() === normalizedName.toLowerCase(),
    );
    if (hasDuplicatedName) {
      form.setError('name', {
        type: 'validate',
        message: t('knowledgeCompilation.nameDuplicated'),
      });
      return;
    }
    await onSubmit({
      name: normalizedName,
      description: values.description || undefined,
      kind: values.kind,
      config: formValuesToTemplateConfig(values),
    });
  });

  return (
    <FormProvider {...form}>
      <Form {...form}>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex items-start gap-3">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem className="flex-1">
                  <FormLabel>{t('knowledgeCompilation.name')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={128}
                      placeholder={t('knowledgeCompilation.namePlaceholder')}
                      autoFocus
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className="pt-7">
              <BuiltinTemplatePopover onSelect={handleSelectBuiltin} />
            </div>
          </div>

          <FormField
            control={form.control}
            name="description"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('knowledgeCompilation.description')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    maxLength={TEXT_FIELD_MAX}
                    placeholder={t(
                      'knowledgeCompilation.descriptionPlaceholder',
                    )}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <EntityRelationSection
            variant="entity"
            kind={kind}
            fieldTemplates={fieldTemplates.entity}
          />
          <EntityRelationSection
            variant="relation"
            kind={kind}
            fieldTemplates={fieldTemplates.relation}
          />

          {kind === 'artifacts' && <ArtifactExtras />}

          <GlobalRulesBlock />

          <div className="flex justify-end gap-2 pt-2 border-t border-border-button">
            <Button type="button" variant="ghost" onClick={onCancel}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={loading}>
              {t('common.save')}
            </Button>
          </div>
        </form>
      </Form>

      <AlertDialog
        open={pendingBuiltin !== null}
        onOpenChange={(open) => {
          if (!open) cancelPendingBuiltin();
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('knowledgeCompilation.confirmSwitchTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('knowledgeCompilation.confirmSwitchBody', {
                name: pendingBuiltin?.display_name ?? '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirmApplyPendingBuiltin}>
              {t('knowledgeCompilation.discardAndSwitch')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </FormProvider>
  );
}
