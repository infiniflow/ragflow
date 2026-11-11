import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { LucideTriangleAlert } from 'lucide-react';

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import message from '@/components/ui/message';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

import { PlatformConfirmDialog } from './confirm-dialog';
import { LoadError } from './load-error';
import { focusWhenLost } from './utils';
import { PlatformDefaultTypes, PlatformModelTypeLabelKeys } from './constants';
import {
  PlatformModelOption,
  useAllPlatformModels,
  usePlatformDefaults,
  useSetPlatformDefault,
} from './hooks';

function isUsable(model: PlatformModelOption) {
  return model.status === 'active' && model.instance_status === 'active';
}

/** Platform default model per type, used when a user has not chosen one. */
export function PlatformDefaultsSection() {
  const { t } = useTranslation();
  const { modelsQuery, providersQuery } = useAllPlatformModels();
  const defaultsQuery = usePlatformDefaults();
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data]);
  const defaults = useMemo(
    () => defaultsQuery.data ?? [],
    [defaultsQuery.data],
  );
  const setDefault = useSetPlatformDefault();
  const [pendingClear, setPendingClear] = useState<string | null>(null);
  // Kept while the confirmation closes, so its subject does not vanish.
  const [shownClear, setShownClear] = useState<string | null>(null);
  const handleClear = useCallback((modelType: string) => {
    setShownClear(modelType);
    setPendingClear(modelType);
  }, []);

  const defaultByType = useMemo(
    () => new Map(defaults.map((item) => [item.model_type, item.model_id])),
    [defaults],
  );
  const modelById = useMemo(
    () => new Map(models.map((model) => [model.model_id, model])),
    [models],
  );

  const handleConfirmClear = useCallback(async () => {
    if (!pendingClear) return;
    try {
      await setDefault.mutateAsync({ modelType: pendingClear, modelId: null });
      message.success(t('message.modified'));
      setPendingClear(null);
      // The clear button hides with the value; focus the row's picker.
      const picker = `[data-testid="platform-default-${pendingClear}"]`;
      focusWhenLost(() => document.querySelector<HTMLElement>(picker));
    } catch {
      // Reported by the admin request layer.
    }
  }, [pendingClear, setDefault, t]);

  const handleClearDialogChange = useCallback(
    (open: boolean) => !open && setPendingClear(null),
    [],
  );

  const providersFailed = providersQuery.isError;
  const handleRetry = useCallback(
    () =>
      providersFailed
        ? providersQuery.refetch()
        : Promise.all([modelsQuery.refetch(), defaultsQuery.refetch()]),
    [modelsQuery, defaultsQuery, providersFailed, providersQuery],
  );

  const loading =
    providersQuery.isLoading ||
    modelsQuery.isLoading ||
    defaultsQuery.isLoading;
  const failed =
    providersFailed || modelsQuery.isError || defaultsQuery.isError;

  return (
    <Card className="!shadow-none bg-transparent">
      <CardHeader>
        <CardTitle className="text-xl leading-none">
          {t('admin.platformModelsPage.defaultsTitle')}
        </CardTitle>
        <CardDescription className="text-text-secondary">
          {t('admin.platformModelsPage.defaultsDescription')}
        </CardDescription>
      </CardHeader>

      <CardContent className="max-w-4xl space-y-4">
        {failed ? (
          <LoadError
            onRetry={handleRetry}
            message={
              defaultsQuery.isError && !modelsQuery.isError
                ? t('admin.platformModelsPage.defaultsLoadFailed')
                : undefined
            }
          />
        ) : (
          PlatformDefaultTypes.map((modelType) => (
            <DefaultRow
              key={modelType}
              modelType={modelType}
              value={defaultByType.get(modelType) ?? ''}
              models={models}
              modelById={modelById}
              loading={loading}
              saving={setDefault.isPending}
              onSave={setDefault.mutateAsync}
              onClear={handleClear}
            />
          ))
        )}
      </CardContent>

      <PlatformConfirmDialog
        open={pendingClear !== null}
        onOpenChange={handleClearDialogChange}
        title={t('admin.platformModelsPage.clearDefaultTitle')}
        description={t('admin.platformModelsPage.clearDefaultDescription')}
        subject={shownClear && t(PlatformModelTypeLabelKeys[shownClear])}
        confirmText={t('admin.platformModelsPage.clearDefault')}
        loading={setDefault.isPending}
        onConfirm={handleConfirmClear}
      />
    </Card>
  );
}

interface DefaultRowProps {
  modelType: string;
  value: string;
  models: PlatformModelOption[];
  modelById: Map<string, PlatformModelOption>;
  loading: boolean;
  saving: boolean;
  onSave: (params: {
    modelType: string;
    modelId: string | null;
  }) => Promise<unknown>;
  onClear: (modelType: string) => void;
}

function DefaultRow({
  modelType,
  value,
  models,
  modelById,
  loading,
  saving,
  onSave,
  onClear,
}: DefaultRowProps) {
  const { t } = useTranslation();
  // Remounting the picker after a failed save shows the saved value again.
  const [attempt, setAttempt] = useState(0);
  const pickerRef = useRef<HTMLButtonElement>(null);
  const label = t(PlatformModelTypeLabelKeys[modelType]);
  const labelId = `platform-default-${modelType}`;
  const warningId = `${labelId}-warning`;
  const current = value ? modelById.get(value) : undefined;
  // A default that tenants cannot use, spelled out under the picker so a
  // narrow picker does not cut it off.
  const warning = !value
    ? undefined
    : !current
      ? t('admin.platformModelsPage.defaultMissing')
      : !isUsable(current) || !current.model_type.includes(modelType)
        ? t('admin.platformModelsPage.defaultUnavailable', {
            name: current.model_name,
          })
        : undefined;
  const options = useMemo(
    () =>
      groupOptions(
        models.filter(
          (model) => isUsable(model) && model.model_type.includes(modelType),
        ),
      ),
    [models, modelType],
  );
  const selectedLabel = useMemo(() => {
    for (const group of options) {
      const option = group.options.find((item) => item.value === value);
      if (option) return option.label;
    }
    return undefined;
  }, [options, value]);

  const handleChange = useCallback(
    async (modelId: string) => {
      // The picker stays focusable while saving; a change then is ignored,
      // and remounting it shows the saved value again instead of the choice.
      if (saving) {
        setAttempt((n) => n + 1);
        focusWhenLost(() => pickerRef.current);
        return;
      }
      if (!modelId) {
        onClear(modelType);
        return;
      }
      try {
        await onSave({ modelType, modelId });
        message.success(t('message.modified'));
      } catch {
        setAttempt((n) => n + 1);
        focusWhenLost(() => pickerRef.current);
      }
    },
    [modelType, onClear, onSave, saving, t],
  );
  const handleClear = useCallback(() => {
    if (!saving) onClear(modelType);
  }, [modelType, onClear, saving]);

  const renderMissing = useCallback(
    (modelId: string) => {
      const model = modelById.get(modelId);
      return (
        <span className="flex items-center gap-1.5 min-w-0 text-state-warning">
          <LucideTriangleAlert className="size-4 shrink-0" />
          {(() => {
            const text = model
              ? t('admin.platformModelsPage.defaultUnavailable', {
                  name: model.model_name,
                })
              : t('admin.platformModelsPage.defaultMissing');
            return (
              <span className="truncate" title={text}>
                {text}
              </span>
            );
          })()}
        </span>
      );
    },
    [modelById, t],
  );

  return (
    <div
      role="group"
      aria-labelledby={labelId}
      aria-describedby={warning ? warningId : undefined}
      // Wraps by available width: the admin sidebar leaves little room on
      // narrow screens, so viewport breakpoints do not fit.
      className="flex flex-wrap items-center gap-x-4 gap-y-2"
    >
      <span id={labelId} className="text-sm text-text-secondary w-32 shrink-0">
        {label}
      </span>
      <div className="flex flex-1 basis-48 min-w-0 gap-2">
        {/* The closed picker truncates; the full name shows on hover. */}
        <div className="flex-1 min-w-0" title={selectedLabel}>
          {loading ? (
            <Skeleton className="h-8 w-full" />
          ) : (
            <SelectWithSearch
              ref={pickerRef}
              key={attempt}
              value={value}
              // The search box holds the keyboard handling of the list.
              alwaysShowSearch
              // Long names wrap inside the viewport instead of widening the
              // list past it.
              contentClassName="max-w-[var(--radix-popover-content-available-width)] [overflow-wrap:anywhere] [&_[cmdk-item]>span]:leading-snug"
              options={options}
              onChange={handleChange}
              placeholder={t('admin.platformModelsPage.noDefault')}
              emptyData={t('admin.platformModelsPage.noModelsOfType')}
              renderMissingValue={renderMissing}
              testId={labelId}
              ariaLabelledBy={labelId}
            />
          )}
        </div>
        {/* Kept in layout while hidden so the pickers stay aligned. */}
        <Button
          type="button"
          variant="ghost"
          className={cn('shrink-0', !value && 'invisible')}
          aria-hidden={!value}
          onClick={handleClear}
          disabled={!value}
          aria-disabled={saving}
          aria-label={t('admin.platformModelsPage.clearDefaultFor', {
            type: label,
          })}
        >
          {t('admin.platformModelsPage.clearDefault')}
        </Button>
      </div>
      {warning && !loading && (
        <p
          id={warningId}
          className="flex basis-full min-w-0 items-start gap-1 text-xs text-text-primary [overflow-wrap:anywhere]"
        >
          <LucideTriangleAlert className="size-3.5 shrink-0 text-state-warning" />
          {warning}
        </p>
      )}
    </div>
  );
}

/**
 * Picker options grouped by provider and instance. A model name that occurs
 * in several instances carries its location, because the closed picker only
 * shows the option label.
 */
function groupOptions(models: PlatformModelOption[]) {
  const nameCount = new Map<string, number>();
  for (const model of models) {
    nameCount.set(model.model_name, (nameCount.get(model.model_name) ?? 0) + 1);
  }
  const groups = new Map<string, PlatformModelOption[]>();
  for (const model of models) {
    const key = `${model.provider_name} / ${model.instance_name}`;
    const group = groups.get(key) ?? [];
    group.push(model);
    groups.set(key, group);
  }
  return Array.from(groups.entries()).map(([label, items]) => ({
    label,
    options: items.map((model) => ({
      label:
        (nameCount.get(model.model_name) ?? 0) > 1
          ? `${model.model_name} (${label})`
          : model.model_name,
      value: model.model_id,
    })),
  }));
}
