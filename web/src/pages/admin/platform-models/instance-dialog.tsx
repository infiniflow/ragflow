import { isEqual } from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  DynamicForm,
  DynamicFormRef,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import { getProviderConfig } from '@/components/model-provider-schema/field-config';
import { useProviderFields } from '@/components/model-provider-schema/hooks';
import { isProviderCredentialField } from '@/components/model-provider-schema/field-config/utils';
import type { SelectOption } from '@/components/model-provider-schema/types';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import message from '@/components/ui/message';
import { cn } from '@/lib/utils';
import { Skeleton } from '@/components/ui/skeleton';

import { BusyButton } from './busy-button';
import { LoadError } from './load-error';

import {
  useCheckPlatformConnection,
  usePlatformInstanceDetail,
  PlatformModelKeys,
  useCloseUnlessSaving,
  useBlockBusyEvents,
  usePlatformProviderCatalog,
  useReturnFocusOnClose,
  useSavePlatformInstance,
} from './hooks';
import {
  baseUrlFieldOf,
  buildInstanceFormValues,
  buildInstanceSubmission,
  hasCredentialInput,
  isHiddenSavedAddress,
  missingTestFields,
  ModelOwnedFieldNames,
  pickConnectionModels,
  pickSafeSettings,
  withClearedSettings,
  withoutUnsavedDefaults,
} from './utils';

const AlwaysRender = () => true;

interface PlatformInstanceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: AdminService.PlatformProvider;
  /** Absent when creating an instance. */
  instance?: AdminService.PlatformInstance;
  /** Models of the edited instance; bounds the connection test. */
  models?: AdminService.PlatformModel[];
  /** False while the edited instance's models are still loading. */
  modelsReady?: boolean;
  /** The edited instance's models failed to load; the test needs them. */
  modelsFailed?: boolean;
  onRetryModels?: () => void;
}

/**
 * Create or edit a platform instance with the same provider form schema as
 * tenant instances. Stored credentials are never shown: a blank credential
 * field keeps the saved value and a typed one replaces it.
 */
export function PlatformInstanceDialog(props: PlatformInstanceDialogProps) {
  const { open, onOpenChange, provider, instance } = props;
  const { t } = useTranslation();
  const returnFocus = useReturnFocusOnClose(open);
  const handleOpenChange = useCloseUnlessSaving(onOpenChange, [
    PlatformModelKeys.saveInstance(provider.id),
    PlatformModelKeys.checkConnection(provider.id),
  ]);
  const {
    data: fetchedDetail,
    isLoading,
    isError,
    refetch,
  } = usePlatformInstanceDetail(provider.id, open ? instance?.id : undefined);
  // The detail is fetched afresh on every open; while the dialog closes it
  // keeps showing the last one instead of falling back to a skeleton.
  const lastDetailRef = useRef(fetchedDetail);
  if (open && fetchedDetail) lastDetailRef.current = fetchedDetail;
  if (open && !fetchedDetail && isLoading) lastDetailRef.current = undefined;
  const detail = open ? fetchedDetail : lastDetailRef.current;
  // The form's fields and defaults depend on the catalog; rendering before
  // it arrives would reset whatever the admin has already typed.
  const catalogQuery = usePlatformProviderCatalog();
  // A new instance has no detail to fetch; refetch() would ignore the
  // query's `enabled` and request an undefined instance.
  const handleRetry = useCallback(
    () =>
      instance
        ? Promise.all([refetch(), catalogQuery.refetch()])
        : catalogQuery.refetch(),
    [instance, refetch, catalogQuery],
  );
  // While closing, keep showing what the dialog showed when it was open.
  const failedNow = Boolean((instance && isError) || catalogQuery.isError);
  const lastFailedRef = useRef(failedNow);
  if (open) lastFailedRef.current = failedNow;
  const failed = open ? failedNow : lastFailedRef.current;
  const loading =
    catalogQuery.isLoading || (instance && ((open && isLoading) || !detail));

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-2xl" onCloseAutoFocus={returnFocus}>
        <DialogHeader>
          <DialogTitle className="min-w-0 [overflow-wrap:anywhere]">
            {instance
              ? t('admin.platformModelsPage.editInstance', {
                  name: instance.instance_name,
                })
              : t('admin.platformModelsPage.addInstanceTo', {
                  name: provider.name,
                })}
          </DialogTitle>
          <DialogDescription>
            {instance
              ? t('admin.platformModelsPage.credentialKeepHint')
              : t('admin.platformModelsPage.instanceDescription')}
          </DialogDescription>
        </DialogHeader>

        {failed ? (
          <LoadError
            onRetry={handleRetry}
            message={
              catalogQuery.isError
                ? t('admin.platformModelsPage.catalogLoadFailed')
                : undefined
            }
          />
        ) : loading ? (
          <div className="px-6 space-y-4 pb-6">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-10 w-full" />
          </div>
        ) : (
          <InstanceForm
            {...props}
            key={detail?.id ?? 'new'}
            detail={instance ? detail : undefined}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function InstanceForm({
  onOpenChange,
  provider,
  instance,
  models,
  modelsReady = true,
  modelsFailed,
  onRetryModels,
  detail,
}: PlatformInstanceDialogProps & {
  detail?: AdminService.PlatformInstanceDetail;
}) {
  const { t } = useTranslation();
  const formRef = useRef<DynamicFormRef>(null);
  const config = useMemo(() => getProviderConfig(provider.name), [provider]);
  const { data: catalog } = usePlatformProviderCatalog();
  const saveMutation = useSavePlatformInstance(provider.id);
  const connectionMutation = useCheckPlatformConnection(provider.id);
  const [connectionResult, setConnectionResult] = useState<
    'success' | 'failed' | null
  >(null);
  // Bumped on every edit so a result never describes values since changed.
  const formVersionRef = useRef(0);
  // Fields the admin changed at least once, even back to the start value.
  const editedFieldsRef = useRef<Set<string>>(new Set());
  const lastValuesRef = useRef<Record<string, unknown> | null>(null);
  useEffect(() => {
    // The form renders with its final start values (the dialog waits for the
    // instance detail), so they are the baseline for "edited".
    lastValuesRef.current = { ...(formRef.current?.getValues() ?? {}) };
    return formRef.current?.watchDirty((_dirty, values) => {
      formVersionRef.current += 1;
      setConnectionResult(null);
      const last = lastValuesRef.current ?? {};
      Object.keys(values ?? {}).forEach((field) => {
        if (!isEqual(values[field], last[field])) {
          editedFieldsRef.current.add(field);
        }
      });
      lastValuesRef.current = { ...values };
    });
  }, []);

  const catalogItem = catalog?.find((item) => item.name === provider.name);
  const baseUrlOptions = useMemo<SelectOption[] | undefined>(() => {
    const urls = catalogItem?.url ?? {};
    const options = Object.entries(urls)
      .filter(([, value]) => Boolean(value))
      .map(([regionKey, value]) => ({
        value: value as string,
        label: value as string,
        regionKey,
      }));
    return options.length > 0 ? options : undefined;
  }, [catalogItem]);
  // Without edit mode the hook seeds a new instance with the field defaults
  // and the catalog's default endpoint of every URL field.
  const {
    fields,
    baseUrlRegionMaps,
    defaultValues: newInstanceValues,
  } = useProviderFields({
    llmFactory: provider.name,
    baseUrlOptions,
    hideWhenInstanceExists: AlwaysRender,
    urlHint: catalogItem?.url_hint,
  });

  const [initialValues] = useState(() => ({
    ...(detail ? buildInstanceFormValues(config, detail) : newInstanceValues),
    instance_name: instance?.instance_name ?? '',
  }));
  const baseUrlField = useMemo(() => baseUrlFieldOf(config), [config]);

  const credentialStatus = detail?.credential_status;
  const formFields = useMemo<FormFieldConfig[]>(() => {
    const nameField: FormFieldConfig = {
      name: 'instance_name',
      label: t('setting.instanceName'),
      type: FormFieldType.Text,
      required: true,
      placeholder: t('setting.instanceNameMessage'),
      tooltip: t('setting.instanceNameTip'),
      autoComplete: 'new-password',
    };
    const providerFields = fields
      .filter((field) => !ModelOwnedFieldNames.has(field.name))
      // Keep browsers from filling the admin login into provider fields.
      .map((field) => ({ ...field, autoComplete: 'new-password' }))
      .map((field) => {
        const isSavedCredential =
          Boolean(detail) &&
          (credentialStatus?.[field.name] === true ||
            (detail?.api_key_configured &&
              isProviderCredentialField(field.name)));
        if (isSavedCredential) {
          return {
            ...field,
            type: FormFieldType.Password,
            required: false,
            autoComplete: 'new-password',
            placeholder: t(
              'admin.platformModelsPage.credentialKeepPlaceholder',
            ),
          };
        }
        if (detail && isHiddenSavedAddress(detail, field.name, baseUrlField)) {
          // The saved address is hidden because it may carry credentials.
          // A plain input shows the keep hint; the URL suggestions would
          // suggest that the catalog default is in use.
          return {
            ...field,
            type: FormFieldType.Text,
            render: undefined,
            required: false,
            placeholder: t('admin.platformModelsPage.baseUrlKeepPlaceholder'),
          };
        }
        return field;
      });
    return [nameField, ...providerFields];
  }, [fields, detail, credentialStatus, baseUrlField, t]);

  const regionByUrl = useMemo(() => {
    const merged = new Map<string, string>();
    Object.values(baseUrlRegionMaps).forEach((map) =>
      map.forEach((region, url) => merged.set(url, region)),
    );
    return merged;
  }, [baseUrlRegionMaps]);

  const readSubmission = useCallback(async () => {
    // Focus the first invalid field, so the error is found and announced.
    const valid = await formRef.current?.trigger(undefined, {
      shouldFocus: true,
    });
    if (!valid) return null;
    const values = (formRef.current?.getFilteredValues() ?? {}) as Record<
      string,
      any
    >;
    return {
      values,
      submission: buildInstanceSubmission(config, values, regionByUrl),
    };
  }, [config, regionByUrl]);

  // The values the edit form started with: saved settings, provider echo
  // defaults and field defaults.
  const filledValues = useMemo(() => {
    const filled: Record<string, unknown> = {};
    formFields.forEach((field) => {
      const value = initialValues[field.name as keyof typeof initialValues];
      filled[field.name] = value !== undefined ? value : field.defaultValue;
    });
    return filled;
  }, [formFields, initialValues]);

  // An edited instance sends only what changed against what is saved:
  // defaults it never stored stay out, settings the admin emptied are
  // removed.
  const savedSettingsChange = useCallback(
    (
      apiKey: string | Record<string, unknown>,
      shownFields: string[],
      values: Record<string, unknown>,
    ) =>
      detail
        ? withClearedSettings(
            withoutUnsavedDefaults(
              apiKey,
              detail,
              filledValues,
              values,
              editedFieldsRef.current,
            ),
            detail,
            shownFields,
            values,
          )
        : apiKey,
    [detail, filledValues],
  );

  const runTestConnection = useCallback(async () => {
    const read = await readSubmission();
    if (!read) return;
    const { values, submission } = read;
    const typedCredentials = hasCredentialInput(values);
    const shownFields = formFields
      .filter((field) => field.shouldRender?.(values) !== false)
      .map((field) => field.name);
    // A changed credential is tested on its own, so blank saved credentials
    // and hidden saved addresses of this form must be entered again.
    if (
      detail &&
      typedCredentials &&
      missingTestFields(detail, shownFields, values).length > 0
    ) {
      message.warning(t('admin.platformModelsPage.enterAllCredentials'));
      return;
    }
    const useSaved = Boolean(instance) && !typedCredentials;
    const modelInfo = pickConnectionModels(models ?? []);
    // With no model to call, the server would test the provider's catalog
    // models instead, which says nothing about this instance's models.
    if (instance && modelInfo.length === 0) {
      message.warning(t('admin.platformModelsPage.noActiveModelToTest'));
      return;
    }
    setConnectionResult(null);
    const version = formVersionRef.current;
    const report = (result: 'success' | 'failed') => {
      if (formVersionRef.current === version) setConnectionResult(result);
    };
    try {
      await connectionMutation.mutateAsync(
        useSaved
          ? {
              instance_id: instance!.id,
              use_saved_credentials: true,
              // Test what saving would store: emptied settings are removed.
              api_key_config: pickSafeSettings(
                savedSettingsChange(submission.apiKey, shownFields, values),
              ),
              base_url: submission.baseUrl || undefined,
              region: submission.region,
              model_info: modelInfo,
            }
          : {
              instance_id: instance?.id,
              api_key: submission.apiKey,
              base_url: submission.baseUrl || undefined,
              region: submission.region,
              model_info: modelInfo,
            },
      );
      report('success');
    } catch {
      report('failed');
    }
  }, [
    readSubmission,
    instance,
    detail,
    formFields,
    models,
    connectionMutation,
    savedSettingsChange,
    t,
  ]);

  const runSave = useCallback(async () => {
    const read = await readSubmission();
    if (!read) return;
    const { submission } = read;
    try {
      await saveMutation.mutateAsync({
        instanceId: instance?.id,
        params: {
          instance_name: submission.instanceName,
          api_key: savedSettingsChange(
            submission.apiKey,
            formFields
              .filter((field) => field.shouldRender?.(read.values) !== false)
              .map((field) => field.name),
            read.values,
          ),
          base_url: submission.baseUrl,
          region: submission.region,
        },
      });
      message.success(t(instance ? 'message.modified' : 'message.created'));
      onOpenChange(false);
    } catch {
      // The admin request layer already reported the error; keep the input.
    }
  }, [
    readSubmission,
    saveMutation,
    instance,
    formFields,
    savedSettingsChange,
    onOpenChange,
    t,
  ]);

  // Saving and testing share one lock, taken before any await, so a second
  // click in the same frame starts nothing.
  const actionRef = useRef(false);
  const [actionRunning, setActionRunning] = useState(false);
  const runExclusive = useCallback(async (action: () => Promise<void>) => {
    if (actionRef.current) return;
    actionRef.current = true;
    setActionRunning(true);
    try {
      await action();
    } finally {
      actionRef.current = false;
      setActionRunning(false);
    }
  }, []);
  const handleTestConnection = useCallback(
    () => runExclusive(runTestConnection),
    [runExclusive, runTestConnection],
  );
  const handleSave = useCallback(
    () => runExclusive(runSave),
    [runExclusive, runSave],
  );

  const busy =
    actionRunning || saveMutation.isPending || connectionMutation.isPending;
  const busyEvents = useBlockBusyEvents(busy);
  const handleCancel = useCallback(() => onOpenChange(false), [onOpenChange]);

  return (
    <>
      <section className="px-6 max-h-[60vh] overflow-y-auto">
        <fieldset disabled={busy} {...busyEvents}>
          <DynamicForm.Root
            ref={formRef}
            fields={formFields}
            defaultValues={initialValues}
            onSubmit={handleSave}
          />
        </fieldset>
        {modelsFailed && onRetryModels && <LoadError onRetry={onRetryModels} />}
        {/* Always present, so screen readers announce the result when its
            text appears. */}
        <p
          role="status"
          className={cn(
            'text-sm',
            connectionResult && 'mt-4',
            connectionResult === 'success'
              ? 'text-state-success'
              : 'text-state-error',
          )}
        >
          {connectionResult === 'success' &&
            t('admin.platformModelsPage.connectionSucceeded')}
          {connectionResult === 'failed' &&
            t('admin.platformModelsPage.connectionFailed')}
        </p>
      </section>

      <DialogFooter className="gap-4 px-6 py-4">
        <BusyButton
          type="button"
          variant="outline"
          className="px-4 h-10 dark:border-border-button"
          onClick={handleTestConnection}
          // Testing before the models load would test catalog models instead.
          disabled={!modelsReady}
          busy={busy}
        >
          {t('admin.platformModelsPage.testConnection')}
        </BusyButton>
        <Button
          type="button"
          variant="outline"
          className="px-4 h-10 dark:border-border-button"
          onClick={handleCancel}
          disabled={busy}
        >
          {t('admin.cancel')}
        </Button>
        <BusyButton
          type="button"
          className="px-4 h-10"
          onClick={handleSave}
          busy={busy}
        >
          {t('admin.confirm')}
        </BusyButton>
      </DialogFooter>
    </>
  );
}
