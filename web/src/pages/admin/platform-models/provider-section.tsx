import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import {
  LucideChevronDown,
  LucideDot,
  LucidePencil,
  LucidePlus,
} from 'lucide-react';

import { LlmIcon } from '@/components/svg-icon';
import { TruncatedText } from '@/components/truncated-text';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import message from '@/components/ui/message';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';

import { PlatformConfirmDialog } from './confirm-dialog';
import { PlatformModelTypeLabelKeys } from './constants';
import {
  usePlatformInstances,
  usePlatformModels,
  useSetPlatformInstanceStatus,
  useSetPlatformModelStatus,
  useSingleFlight,
} from './hooks';
import { PlatformInstanceDialog } from './instance-dialog';
import { focusWhenLost } from './utils';
import { LoadError } from './load-error';
import { PlatformModelDialog } from './model-dialog';

export function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const active = status === 'active';
  return (
    <Badge variant={active ? 'success' : 'destructive'} className="shrink-0">
      <LucideDot className="size-[1em] stroke-[8] mr-1" />
      {active
        ? t('admin.platformModelsPage.statusActive')
        : t('admin.platformModelsPage.statusInactive')}
    </Badge>
  );
}

type StatusTarget = { kind: 'instance' | 'model'; id: string; name: string };

/** One platform provider: its instances, and each instance's models. */
export function ProviderSection({
  provider,
}: {
  provider: AdminService.PlatformProvider;
}) {
  const { t } = useTranslation();
  const {
    data: instances = [],
    isLoading,
    isError,
    refetch,
  } = usePlatformInstances(provider.id);
  const [createOpen, setCreateOpen] = useState(false);
  const handleCreate = useCallback(() => setCreateOpen(true), []);
  const handleRetry = useCallback(() => refetch(), [refetch]);

  return (
    <section
      aria-labelledby={`platform-provider-${provider.id}`}
      className="rounded-lg border-0.5 border-border-button p-4 space-y-4"
    >
      <header className="flex flex-wrap items-center gap-3 min-w-0">
        <LlmIcon
          name={provider.name}
          width={24}
          height={24}
          imgClass="size-6 shrink-0"
        />
        <h3
          id={`platform-provider-${provider.id}`}
          className="text-base font-medium min-w-[8rem] flex-1"
        >
          <TruncatedText className="truncate" tooltip={provider.name}>
            {provider.name}
          </TruncatedText>
        </h3>
        <Button
          variant="outline"
          className="h-9 shrink-0 dark:border-border-button"
          onClick={handleCreate}
        >
          <LucidePlus />
          {t('admin.platformModelsPage.addInstance')}
        </Button>
      </header>

      {isError ? (
        <LoadError onRetry={handleRetry} />
      ) : isLoading ? (
        <Skeleton className="h-12 w-full" />
      ) : instances.length === 0 ? (
        <p className="text-sm text-text-secondary">
          {t('admin.platformModelsPage.noInstances')}
        </p>
      ) : (
        <ul className="space-y-3">
          {instances.map((instance, index) => (
            <li key={instance.id}>
              <InstanceItem
                provider={provider}
                instance={instance}
                defaultOpen={index === 0}
              />
            </li>
          ))}
        </ul>
      )}

      <PlatformInstanceDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        provider={provider}
      />
    </section>
  );
}

function InstanceItem({
  provider,
  instance,
  defaultOpen,
}: {
  provider: AdminService.PlatformProvider;
  instance: AdminService.PlatformInstance;
  defaultOpen: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(defaultOpen);
  const [editOpen, setEditOpen] = useState(false);
  const [modelDialog, setModelDialog] = useState<{
    open: boolean;
    model?: AdminService.PlatformModel;
  }>({ open: false });
  const [pendingStatus, setPendingStatus] = useState<StatusTarget | null>(null);
  // What the confirmation shows; kept while it closes so its text does not
  // change during the closing animation.
  const [shownStatus, setShownStatus] = useState<StatusTarget | null>(null);

  // Models are needed by the table and by the edit dialog's connection test.
  const {
    data: models = [],
    isLoading: modelsLoading,
    isError: modelsError,
    refetch: refetchModels,
  } = usePlatformModels(provider.id, instance.id, open || editOpen);
  const instanceStatus = useSetPlatformInstanceStatus(provider.id);
  const modelStatus = useSetPlatformModelStatus(provider.id, instance.id);
  const existingNames = useMemo(
    () => models.map((model) => model.model_name),
    [models],
  );

  const applyStatus = useCallback(
    async (target: StatusTarget, status: AdminService.PlatformStatus) => {
      try {
        if (target.kind === 'instance') {
          await instanceStatus.mutateAsync({ instanceId: target.id, status });
        } else {
          await modelStatus.mutateAsync({ modelId: target.id, status });
        }
        message.success(t('message.modified'));
        setPendingStatus(null);
        // The confirmation was not opened from its button, so it cannot hand
        // focus back; return it to the toggle that now offers the opposite.
        const label = t(
          status === 'active'
            ? `admin.platformModelsPage.disable${target.kind === 'instance' ? 'Instance' : 'Model'}`
            : `admin.platformModelsPage.enable${target.kind === 'instance' ? 'Instance' : 'Model'}`,
          { name: target.name },
        );
        focusWhenLost(() =>
          document.querySelector<HTMLElement>(
            `button[aria-label="${CSS.escape(label)}"]`,
          ),
        );
      } catch {
        // Reported by the admin request layer.
      }
    },
    [instanceStatus, modelStatus, t],
  );

  const statusBusy = instanceStatus.isPending || modelStatus.isPending;
  // Enabling has no confirmation, so it takes the same synchronous lock the
  // confirmation uses; a second click in the same frame starts nothing.
  const enableOnce = useSingleFlight(
    useCallback(
      (target: StatusTarget) => applyStatus(target, 'active'),
      [applyStatus],
    ),
  );
  /** Disabling asks for confirmation; enabling takes effect at once. */
  const toggleStatus = useCallback(
    (target: StatusTarget, active: boolean) => {
      // Buttons stay focusable while a change runs; another is ignored.
      if (statusBusy) return;
      if (active) {
        setShownStatus(target);
        setPendingStatus(target);
      } else {
        enableOnce(target);
      }
    },
    [enableOnce, statusBusy],
  );

  const instanceActive = instance.status === 'active';

  const handleEdit = useCallback(() => setEditOpen(true), []);
  const handleToggleInstance = useCallback(
    () =>
      toggleStatus(
        { kind: 'instance', id: instance.id, name: instance.instance_name },
        instanceActive,
      ),
    [toggleStatus, instance.id, instance.instance_name, instanceActive],
  );
  const handleAddModel = useCallback(() => setModelDialog({ open: true }), []);
  const handleEditModel = useCallback(
    (model: AdminService.PlatformModel) =>
      setModelDialog({ open: true, model }),
    [],
  );
  const handleModelDialogChange = useCallback(
    (next: boolean) => setModelDialog((prev) => ({ ...prev, open: next })),
    [],
  );
  const handleConfirmDialogChange = useCallback(
    (next: boolean) => !next && setPendingStatus(null),
    [],
  );
  const handleConfirmDisable = useCallback(
    () => pendingStatus && applyStatus(pendingStatus, 'inactive'),
    [pendingStatus, applyStatus],
  );
  const handleRetryModels = useCallback(() => refetchModels(), [refetchModels]);

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="rounded-lg bg-bg-card"
    >
      <div className="flex flex-wrap items-center gap-2 px-3 py-2 min-w-0">
        <CollapsibleTrigger asChild>
          <Button
            variant="ghost"
            className="flex-1 min-w-[8rem] justify-start gap-2 px-2"
            aria-label={t('admin.platformModelsPage.toggleInstance', {
              name: instance.instance_name,
            })}
          >
            <LucideChevronDown
              className={cn(
                'size-4 shrink-0 transition-transform',
                !open && '-rotate-90',
              )}
            />
            <span
              className="min-w-0 truncate font-medium"
              title={instance.instance_name}
            >
              {instance.instance_name}
            </span>
          </Button>
        </CollapsibleTrigger>
        <StatusBadge status={instance.status} />
        <Button
          variant="ghost"
          size="icon"
          onClick={handleEdit}
          aria-label={t('admin.platformModelsPage.editInstance', {
            name: instance.instance_name,
          })}
        >
          <LucidePencil />
        </Button>
        <Button
          variant="ghost"
          className="shrink-0"
          aria-disabled={statusBusy}
          onClick={handleToggleInstance}
          aria-label={t(
            instanceActive
              ? 'admin.platformModelsPage.disableInstance'
              : 'admin.platformModelsPage.enableInstance',
            { name: instance.instance_name },
          )}
        >
          {instanceActive
            ? t('admin.platformModelsPage.disable')
            : t('admin.platformModelsPage.enable')}
        </Button>
      </div>

      <CollapsibleContent className="px-3 pb-3 space-y-3">
        <div className="flex flex-wrap items-center justify-end gap-3">
          {!instanceActive && (
            <p className="mr-auto text-sm text-state-warning">
              {t('admin.platformModelsPage.instanceDisabledHint')}
            </p>
          )}
          <Button
            variant="outline"
            className="h-9 dark:border-border-button"
            onClick={handleAddModel}
          >
            <LucidePlus />
            {t('admin.platformModelsPage.addModel')}
          </Button>
        </div>
        {modelsError ? (
          <LoadError onRetry={handleRetryModels} />
        ) : (
          // A list rather than a table: on narrow screens a row wraps, so the
          // status and actions move under the name instead of off-screen.
          <ul
            aria-label={t('admin.platformModelsPage.modelsOf', {
              name: instance.instance_name,
            })}
            className="rounded-2xl bg-bg-card divide-y-0.5 divide-border-button"
          >
            {modelsLoading ? (
              <li className="px-4 py-3">
                <Skeleton className="h-6 w-full" />
              </li>
            ) : models.length === 0 ? (
              <li className="px-4 py-6 text-center text-sm text-text-secondary">
                {t('common.noResults')}
              </li>
            ) : (
              models.map((model) => (
                <ModelRow
                  key={model.model_id}
                  model={model}
                  statusBusy={statusBusy}
                  onEdit={handleEditModel}
                  onToggleStatus={toggleStatus}
                />
              ))
            )}
          </ul>
        )}
      </CollapsibleContent>

      <PlatformInstanceDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        provider={provider}
        instance={instance}
        models={models}
        modelsReady={!modelsLoading && !modelsError}
        modelsFailed={modelsError}
        onRetryModels={handleRetryModels}
      />
      <PlatformModelDialog
        open={modelDialog.open}
        onOpenChange={handleModelDialogChange}
        provider={provider}
        instance={instance}
        model={modelDialog.model}
        existingNames={existingNames}
      />
      <PlatformConfirmDialog
        open={pendingStatus !== null}
        onOpenChange={handleConfirmDialogChange}
        title={
          shownStatus?.kind === 'instance'
            ? t('admin.platformModelsPage.disableInstanceTitle')
            : t('admin.platformModelsPage.disableModelTitle')
        }
        description={t('admin.platformModelsPage.disableDescription')}
        subject={shownStatus?.name}
        confirmText={t('admin.platformModelsPage.disable')}
        loading={statusBusy}
        onConfirm={handleConfirmDisable}
      />
    </Collapsible>
  );
}

function ModelRow({
  model,
  statusBusy,
  onEdit,
  onToggleStatus,
}: {
  model: AdminService.PlatformModel;
  statusBusy: boolean;
  onEdit: (model: AdminService.PlatformModel) => void;
  onToggleStatus: (target: StatusTarget, active: boolean) => void;
}) {
  const { t } = useTranslation();
  const active = model.status === 'active';
  const handleEdit = useCallback(() => onEdit(model), [onEdit, model]);
  const handleToggle = useCallback(
    () =>
      onToggleStatus(
        { kind: 'model', id: model.model_id, name: model.model_name },
        active,
      ),
    [onToggleStatus, model.model_id, model.model_name, active],
  );

  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 min-w-0">
      <div className="flex-1 basis-32 min-w-0">
        <TruncatedText
          className="truncate text-sm font-medium"
          tooltip={model.model_name}
        >
          {model.model_name}
        </TruncatedText>
        <ul
          className="mt-1.5 flex flex-wrap gap-1"
          aria-label={t('setting.modelType')}
        >
          {model.model_type.map((type) => (
            <li key={type}>
              <Badge variant="secondary">
                {t(PlatformModelTypeLabelKeys[type] ?? type)}
              </Badge>
            </li>
          ))}
        </ul>
      </div>
      <div className="flex flex-wrap items-center gap-1 min-w-0">
        <StatusBadge status={model.status} />
        <Button
          variant="ghost"
          size="icon"
          onClick={handleEdit}
          aria-label={t('admin.platformModelsPage.editModel', {
            name: model.model_name,
          })}
        >
          <LucidePencil />
        </Button>
        <Button
          variant="ghost"
          aria-disabled={statusBusy}
          onClick={handleToggle}
          aria-label={t(
            active
              ? 'admin.platformModelsPage.disableModel'
              : 'admin.platformModelsPage.enableModel',
            { name: model.model_name },
          )}
        >
          {active
            ? t('admin.platformModelsPage.disable')
            : t('admin.platformModelsPage.enable')}
        </Button>
      </div>
    </li>
  );
}
