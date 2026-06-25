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
import message from '@/components/ui/message';
import { useFetchBuiltinCompilationTemplates } from '@/hooks/use-compilation-template-request';
import { useFetchDefaultModelDictionary } from '@/hooks/use-llm-request';
import { CompilationTemplate } from '@/interfaces/database/compilation-template';
import { LLMSelect } from '@/pages/dataset/dataset-setting/configuration/common-item';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect, useMemo } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { ArtifactExtras } from './components/artifact-extras';
import { BuiltinTemplatePopover } from './components/builtin-template-popover';
import { EntityRelationSection } from './components/entity-relation-section';
import { GlobalRulesBlock } from './components/global-rules-block';
import { TreeExtras } from './components/tree-extras';
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

/**
 * Walk RHF's nested ``FieldErrors`` tree and return the first
 * ``message`` string we find. Used by the form's ``onInvalid`` toast so
 * the user sees a concrete reason a save was rejected — RHF otherwise
 * silently no-ops, which presents as "Save did nothing."
 */
function _firstFormErrorMessage(errors: unknown): string | undefined {
  if (!errors || typeof errors !== 'object') return undefined;
  const stack: unknown[] = [errors];
  while (stack.length) {
    const node = stack.pop();
    if (!node || typeof node !== 'object') continue;
    const rec = node as Record<string, unknown>;
    if (typeof rec.message === 'string' && rec.message) {
      return rec.message;
    }
    for (const value of Object.values(rec)) {
      if (value && typeof value === 'object') {
        stack.push(value);
      }
    }
  }
  return undefined;
}

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
  // Default chat model from /v1/models/default — used as the prefill for
  // ``llm_id`` when the user opens the form to create a fresh template
  // or edits a legacy one whose config predates the field.
  const defaultModelDict = useFetchDefaultModelDictionary();

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

  // Prefill llm_id from the tenant default whenever the current value
  // is empty — covers initial mount, ``form.reset`` after picking a
  // built-in template, and the async arrival of /v1/models/default.
  // ``form.watch`` re-runs this whenever ``llm_id`` changes, so an
  // explicit user pick (or a saved template with its own ``llm_id``)
  // never gets clobbered.
  const watchedLlmId = form.watch('llm_id');
  useEffect(() => {
    const fallback = defaultModelDict.llm_id;
    if (!fallback) return;
    if (watchedLlmId) return;
    form.setValue('llm_id', fallback, { shouldDirty: false });
  }, [defaultModelDict.llm_id, watchedLlmId, form]);

  // Prefill the artifact ``example`` textarea with the canonical
  // default from the artifacts built-in (which carries
  // ``ARTIFACT_TEMPLATE_EXAMPLE`` verbatim in its YAML). Fires only
  // when ``kind === 'artifacts'`` and the textarea is currently
  // empty — so explicit user edits and saved overrides are never
  // clobbered. The built-in list arrives async, so this useEffect
  // re-runs when ``builtins`` lands.
  const watchedKind = form.watch('kind');
  const watchedExample = form.watch('example');
  useEffect(() => {
    if (watchedKind !== 'artifacts') return;
    if (watchedExample && watchedExample.trim()) return;
    const artifactsBuiltin = builtins.find((b) => b.kind === 'artifacts');
    const def = (artifactsBuiltin?.config as { example?: string } | undefined)
      ?.example;
    if (def && def.trim()) {
      form.setValue('example', def, { shouldDirty: false });
    }
  }, [watchedKind, watchedExample, builtins, form]);

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

  const handleSubmit = form.handleSubmit(
    async (values) => {
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
    },
    // Surface validation failures — without this, RHF silently no-ops the
    // submit when any field rejects, which looks like a broken button.
    (errors) => {
      const firstError = _firstFormErrorMessage(errors);
      if (firstError) {
        message.error(firstError);
      } else {
        message.error(t('knowledgeCompilation.formInvalid'));
      }
    },
  );

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

          <FormField
            control={form.control}
            name="llm_id"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('knowledgeCompilation.llmLabel')}</FormLabel>
                <FormControl>
                  <LLMSelect isEdit field={field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          {kind !== 'tree' && (
            <>
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
            </>
          )}

          {kind === 'artifacts' && <ArtifactExtras />}
          {kind === 'tree' && <TreeExtras />}

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
