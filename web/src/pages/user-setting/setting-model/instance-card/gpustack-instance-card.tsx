/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  useAddInstanceModel,
  useAddProviderInstance,
  useDeleteInstanceModels,
  useFetchInstanceModels,
  useListProviderModels,
  usePatchInstanceModel,
  useUpdateProviderInstance,
} from '@/hooks/use-llm-request';
import {
  ChevronDown,
  ChevronRight,
  CircleCheck,
  CircleAlert,
  ExternalLink,
  Loader2,
  Minus,
  RefreshCw,
  ServerOff,
  Trash2,
} from 'lucide-react';
import {
  ChangeEvent,
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useFetchInstanceNameSet } from '../hooks';
import {
  GPUSTACK_MODEL_KINDS,
  GPUStackModel,
  GPUStackModelKind,
  classifyConnectionError,
  diffModels,
  groupModelsByKind,
  isLoopbackUrl,
  modelTypesFromKind,
  toGPUStackModel,
} from './gpustack-instance-utils';
import { useDeleteInstance, useLazyInstanceDetails } from './hooks';
import {
  ProviderInstanceCardProps,
  ProviderInstanceCardRef,
} from './interface';

const GUIDE_URL =
  'https://docs.gpustack.ai/latest/integrations/integrate-with-ragflow/';
const URL_PLACEHOLDER = 'http://host.docker.internal:80';

type TranslateFn = (key: string, options?: Record<string, unknown>) => string;

function ConnectionError({ message, t }: { message: string; t: TranslateFn }) {
  const kind = classifyConnectionError(message);
  const text =
    kind === 'auth'
      ? t('gpustack.errorAuth')
      : kind === 'unreachable'
        ? t('gpustack.errorUnreachable')
        : t('gpustack.errorOther', { message });
  return (
    <div
      role="alert"
      className="flex items-start gap-2 text-sm text-state-error"
      data-testid="gpustack-error"
    >
      <CircleAlert className="size-4 mt-0.5 shrink-0" />
      <span>{text}</span>
    </div>
  );
}

function EmptyModels({ t }: { t: TranslateFn }) {
  return (
    <div
      className="flex flex-col items-center gap-2 rounded-lg border border-border-button p-6 text-center"
      data-testid="gpustack-empty"
    >
      <ServerOff className="size-5 text-text-secondary" />
      <div className="text-sm font-medium text-text-primary">
        {t('gpustack.emptyTitle')}
      </div>
      <div className="text-sm text-text-secondary max-w-prose">
        {t('gpustack.emptyBody')}
      </div>
      <a
        href={GUIDE_URL}
        target="_blank"
        rel="noreferrer"
        className="inline-flex items-center gap-1 text-sm text-accent-primary"
      >
        {t('gpustack.emptyLink')}
        <ExternalLink className="size-3.5" />
      </a>
    </div>
  );
}

interface ModelGroupsProps {
  models: (GPUStackModel & { running?: boolean })[];
  selected?: Set<string>;
  onToggle?: (name: string) => void;
  onKindChange: (name: string, kind: GPUStackModelKind) => void;
  onRemove?: (name: string) => void;
  t: TranslateFn;
}

interface ModelRowProps extends Omit<ModelGroupsProps, 'models'> {
  model: GPUStackModel & { running?: boolean };
}

function ModelRow({
  model,
  selected,
  onToggle,
  onKindChange,
  onRemove,
  t,
}: ModelRowProps) {
  const handleToggle = useCallback(
    () => onToggle?.(model.name),
    [model.name, onToggle],
  );
  const handleKindChange = useCallback(
    (kind: string) => onKindChange(model.name, kind as GPUStackModelKind),
    [model.name, onKindChange],
  );
  const handleRemove = useCallback(
    () => onRemove?.(model.name),
    [model.name, onRemove],
  );

  return (
    <li className="flex items-center gap-3 px-3 py-2">
      {selected && onToggle && (
        <Checkbox
          checked={selected.has(model.name)}
          onCheckedChange={handleToggle}
          aria-label={t('gpustack.select', { name: model.name })}
        />
      )}
      <span className="min-w-0 flex-1 truncate text-sm text-text-primary">
        {model.name}
      </span>
      {model.running === false && (
        <span
          className="shrink-0 rounded px-1.5 py-0.5 text-xs text-state-warning border border-state-warning"
          title={t('gpustack.notRunningTip')}
        >
          {t('gpustack.notRunning')}
        </span>
      )}
      {model.kind !== 'asr' && model.kind !== 'tts' && (
        <span className="shrink-0 text-xs text-text-secondary">
          {t('gpustack.tokens', {
            count: model.maxTokens.toLocaleString(),
          })}
        </span>
      )}
      <Select value={model.kind} onValueChange={handleKindChange}>
        <SelectTrigger
          className="w-40 shrink-0"
          aria-label={t('gpustack.changeType', { name: model.name })}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {GPUSTACK_MODEL_KINDS.map((kind) => (
            <SelectItem key={kind} value={kind}>
              {t(`gpustack.kind_${kind}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {onRemove && (
        <ConfirmDeleteDialog onOk={handleRemove}>
          <Button
            variant="ghost"
            size="icon-sm"
            className="shrink-0"
            aria-label={t('gpustack.remove', { name: model.name })}
          >
            <Minus className="size-4" />
          </Button>
        </ConfirmDeleteDialog>
      )}
    </li>
  );
}

function ModelGroups({
  models,
  selected,
  onToggle,
  onKindChange,
  onRemove,
  t,
}: ModelGroupsProps) {
  return (
    <div className="flex flex-col gap-4">
      {groupModelsByKind(models).map((group) => (
        <section
          key={group.kind}
          data-testid={`gpustack-group-${group.kind}`}
          className="flex flex-col gap-1.5"
        >
          <h4 className="text-xs font-medium text-text-secondary">
            {t(`gpustack.kind_${group.kind}`)} ({group.models.length})
          </h4>
          <ul className="rounded-lg border border-border-button divide-y divide-border-button">
            {group.models.map((model) => (
              <ModelRow
                key={model.name}
                model={model}
                selected={selected}
                onToggle={onToggle}
                onKindChange={onKindChange}
                onRemove={onRemove}
                t={t}
              />
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function toModelInfo(model: GPUStackModel) {
  return {
    model_name: model.name,
    model_type: modelTypesFromKind(model.kind),
    max_tokens: model.maxTokens,
  };
}

/**
 * Instance card for GPUStack. A GPUStack server already knows which models it
 * runs and what they are, so connecting is the whole setup: enter the server
 * and key, review what was found, add it.
 */
export const GPUStackInstanceCard = forwardRef<
  ProviderInstanceCardRef,
  ProviderInstanceCardProps
>(function GPUStackInstanceCard(
  { providerName, instance, isDraft = false, onDelete, onSaved, defaultOpen },
  ref,
) {
  const { t } = useTranslation('translation', { keyPrefix: 'setting' });
  const [open, setOpen] = useState(isDraft || !!defaultOpen);
  const [editing, setEditing] = useState(false);
  const [baseUrl, setBaseUrl] = useState('');
  const [apiKey, setApiKey] = useState('');
  const [instanceName, setInstanceName] = useState('');
  const [discovered, setDiscovered] = useState<GPUStackModel[] | null>(null);
  const [error, setError] = useState('');
  const [deselected, setDeselected] = useState<Set<string>>(new Set());
  const [kindOverrides, setKindOverrides] = useState<
    Record<string, GPUStackModelKind>
  >({});
  const [saving, setSaving] = useState(false);

  const { instanceNameSet } = useFetchInstanceNameSet(providerName);
  const { instanceDetails } = useLazyInstanceDetails(
    providerName,
    instance.id,
    isDraft,
    open,
  );
  const {
    data: instanceModels,
    loading: instanceModelsLoading,
    isSuccess: instanceModelsLoaded,
    isFetchedAfterMount: instanceModelsFetched,
  } = useFetchInstanceModels(
    providerName,
    isDraft ? '' : instance.instance_name,
  );
  const { loading: discovering, listProviderModels } = useListProviderModels();
  const { addProviderInstance } = useAddProviderInstance();
  const { updateProviderInstance } = useUpdateProviderInstance();
  const { addInstanceModel } = useAddInstanceModel();
  const { deleteInstanceModels } = useDeleteInstanceModels();
  const { patchInstanceModel } = usePatchInstanceModel();
  const handleDelete = useDeleteInstance(
    providerName,
    instance.instance_name,
    isDraft,
    onDelete,
  );

  const savedBaseUrl = instanceDetails?.base_url ?? '';
  useEffect(() => {
    if (!isDraft && savedBaseUrl) setBaseUrl(savedBaseUrl);
  }, [isDraft, savedBaseUrl]);

  const trimmedName = instanceName.trim();
  const nameTaken = instanceNameSet.has(trimmedName);
  const nameValid = !!trimmedName && !nameTaken;

  // Bumped when the inputs change, so a slower response for the old ones is
  // dropped instead of being saved with the new credentials.
  const discoverRequest = useRef(0);
  const discover = useCallback(async () => {
    const request = ++discoverRequest.current;
    setError('');
    try {
      const ret = await listProviderModels({
        provider_name: providerName,
        skipGlobalErrorNotification: true,
        ...(isDraft
          ? { api_key: apiKey, base_url: baseUrl.trim() }
          : {
              use_saved_credentials: true,
              instance_name: instance.instance_name,
            }),
      });
      if (request !== discoverRequest.current) return;
      if (ret?.code !== 0) {
        setDiscovered(null);
        setError(ret?.message || String(ret?.code));
        return;
      }
      setDiscovered((ret.data ?? []).map(toGPUStackModel));
    } catch (e) {
      if (request !== discoverRequest.current) return;
      setDiscovered(null);
      setError(e instanceof Error ? e.message : String(e));
    }
  }, [
    apiKey,
    baseUrl,
    instance.instance_name,
    isDraft,
    listProviderModels,
    providerName,
  ]);

  // A saved connection always opens showing what the server serves right now.
  const discoveredOnOpen = useRef(false);
  useEffect(() => {
    if (isDraft || !open || discoveredOnOpen.current) return;
    discoveredOnOpen.current = true;
    discover();
  }, [discover, isDraft, open]);

  const withKindOverride = useCallback(
    (model: GPUStackModel) => ({
      ...model,
      kind: kindOverrides[model.name] ?? model.kind,
    }),
    [kindOverrides],
  );
  const { notAdded, added } = useMemo(
    () =>
      diffModels(
        (discovered ?? []).map(withKindOverride),
        (instanceModels ?? []).map((model) =>
          toGPUStackModel({ ...model, model_types: model.model_type }),
        ),
      ),
    [discovered, instanceModels, withKindOverride],
  );
  const selectedModels = notAdded.filter((m) => !deselected.has(m.name));
  const selectedNames = useMemo(
    () => new Set(selectedModels.map((m) => m.name)),
    [selectedModels],
  );

  const toggle = useCallback((name: string) => {
    setDeselected((prev) => {
      const next = new Set(prev);
      if (!next.delete(name)) next.add(name);
      return next;
    });
  }, []);
  const setKind = useCallback((name: string, kind: GPUStackModelKind) => {
    setKindOverrides((prev) => ({ ...prev, [name]: kind }));
  }, []);

  const getSavePayload = useCallback(() => {
    if (!isDraft || !nameValid || discovering || !selectedModels.length) {
      return null;
    }
    return {
      payload: {
        llm_factory: providerName,
        instance_name: trimmedName,
        api_key: apiKey,
        base_url: baseUrl.trim(),
        model_info: selectedModels.map(toModelInfo),
      },
      instanceName: trimmedName,
      isDraft: true,
      apiKind: 'add' as const,
    };
  }, [
    apiKey,
    baseUrl,
    discovering,
    isDraft,
    nameValid,
    providerName,
    selectedModels,
    trimmedName,
  ]);

  useImperativeHandle(
    ref,
    () => ({
      validate: async () => !!getSavePayload(),
      getSavePayload,
      markSaved: () => {},
    }),
    [getSavePayload],
  );

  const addConnection = useCallback(async () => {
    const save = getSavePayload();
    if (!save) return;
    setSaving(true);
    try {
      const ret = await addProviderInstance(save.payload as any);
      if (ret?.code === 0) onSaved?.(save);
    } finally {
      setSaving(false);
    }
  }, [addProviderInstance, getSavePayload, onSaved]);

  const addToSaved = useCallback(async () => {
    setSaving(true);
    try {
      await Promise.all(
        selectedModels.map((model) =>
          addInstanceModel({
            provider_name: providerName,
            instance_name: instance.instance_name,
            model_name: model.name,
            model_type: modelTypesFromKind(model.kind),
            max_tokens: model.maxTokens,
          } as any),
        ),
      );
    } finally {
      setSaving(false);
    }
  }, [addInstanceModel, instance.instance_name, providerName, selectedModels]);

  const saveConnection = useCallback(async () => {
    setSaving(true);
    try {
      const ret = await updateProviderInstance({
        provider_name: providerName,
        id: instance.id,
        instance_name: instance.instance_name,
        base_url: baseUrl.trim(),
        ...(apiKey ? { api_key: apiKey } : {}),
        region: instanceDetails?.region || 'default',
        // Required by the endpoint: the instance's models stay as they are.
        model_info: (instanceModels ?? []).map((model) => ({
          model_name: model.name,
          model_type: model.model_type,
          max_tokens: model.max_tokens,
          ...(model.extra ? { extra: model.extra } : {}),
        })),
        verify: false,
      });
      if (ret?.code === 0) {
        setEditing(false);
        setApiKey('');
        await discover();
      }
    } finally {
      setSaving(false);
    }
  }, [
    apiKey,
    baseUrl,
    discover,
    instance.id,
    instance.instance_name,
    instanceDetails?.region,
    instanceModels,
    providerName,
    updateProviderInstance,
  ]);

  const removeModel = useCallback(
    (name: string) =>
      deleteInstanceModels({
        provider_name: providerName,
        instance_name: instance.instance_name,
        model_name: [name],
      }),
    [deleteInstanceModels, instance.instance_name, providerName],
  );
  const changeSavedKind = useCallback(
    (name: string, kind: GPUStackModelKind) => {
      const saved = instanceModels?.find((model) => model.name === name);
      return patchInstanceModel({
        provider_name: providerName,
        instance_name: instance.instance_name,
        model_name: name,
        max_tokens: saved?.max_tokens ?? 0,
        model_type: modelTypesFromKind(kind),
        extra: saved?.extra ?? {},
      });
    },
    [instance.instance_name, instanceModels, patchInstanceModel, providerName],
  );

  const resetDiscovery = useCallback(() => {
    discoverRequest.current += 1;
    setDiscovered(null);
    setError('');
    setDeselected(new Set());
    setKindOverrides({});
  }, []);

  const handleBaseUrlChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      setBaseUrl(e.target.value);
      if (isDraft) resetDiscovery();
    },
    [isDraft, resetDiscovery],
  );
  const handleApiKeyChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      setApiKey(e.target.value);
      if (isDraft) resetDiscovery();
    },
    [isDraft, resetDiscovery],
  );
  const handleNameChange = useCallback((e: ChangeEvent<HTMLInputElement>) => {
    setInstanceName(e.target.value);
  }, []);
  const toggleOpen = useCallback(() => setOpen((value) => !value), []);
  const toggleEditing = useCallback(() => setEditing((value) => !value), []);

  // Saving sends the full model list and the server deletes anything missing
  // from it, so never save the placeholder list used before the models load.
  // That list counts as a success, and a paused fetch offline is not "loading".
  const instanceModelsReady =
    instanceModelsFetched && instanceModelsLoaded && !instanceModelsLoading;

  const connectionForm = (
    <div className="flex flex-col gap-3">
      <label className="flex flex-col gap-1.5 text-sm font-medium text-text-primary">
        {t('gpustack.serverUrl')}
        <Input
          value={baseUrl}
          onChange={handleBaseUrlChange}
          placeholder={URL_PLACEHOLDER}
          autoComplete="off"
          data-testid="gpustack-base-url"
        />
        <span
          className={
            isLoopbackUrl(baseUrl)
              ? 'text-xs font-normal text-state-warning'
              : 'text-xs font-normal text-text-secondary'
          }
        >
          {isLoopbackUrl(baseUrl)
            ? t('gpustack.loopbackWarning')
            : t('gpustack.serverUrlTip')}
        </span>
      </label>
      <label className="flex flex-col gap-1.5 text-sm font-medium text-text-primary">
        {t('gpustack.apiKey')}
        <Input
          type="password"
          value={apiKey}
          onChange={handleApiKeyChange}
          placeholder={isDraft ? '' : t('gpustack.apiKeyKeep')}
          autoComplete="new-password"
          data-testid="gpustack-api-key"
        />
        <span className="text-xs font-normal text-text-secondary">
          {t('gpustack.apiKeyTip')}
        </span>
      </label>
    </div>
  );

  if (isDraft) {
    const canConnect =
      nameValid && !!baseUrl.trim() && !!apiKey && !discovering;
    return (
      <div
        className="rounded-lg bg-bg-card p-5 mb-5 flex flex-col gap-4"
        data-testid="instance-card-draft"
      >
        <label className="flex flex-col gap-1.5 text-sm font-medium text-text-primary">
          <span>
            <span className="text-state-error mr-0.5">*</span>
            {t('instanceName')}
          </span>
          <Input
            value={instanceName}
            onChange={handleNameChange}
            placeholder={t('instanceNamePlaceholder')}
            autoComplete="off"
            data-testid="gpustack-instance-name"
          />
          {nameTaken && (
            <span className="text-xs font-normal text-state-error">
              {t('instanceNameExists')}
            </span>
          )}
        </label>
        {connectionForm}
        <div className="flex items-center gap-2">
          <Button
            onClick={discover}
            disabled={!canConnect}
            data-testid="gpustack-connect"
          >
            {discovering && <Loader2 className="size-4 animate-spin" />}
            {discovering ? t('gpustack.connecting') : t('gpustack.connect')}
          </Button>
          {onDelete && (
            <Button variant="ghost" onClick={onDelete}>
              {t('gpustack.cancel')}
            </Button>
          )}
        </div>
        {error && <ConnectionError message={error} t={t} />}
        {discovered && discovered.length === 0 && <EmptyModels t={t} />}
        {discovered && discovered.length > 0 && (
          <>
            <div
              className="flex items-center gap-2 text-sm text-state-success"
              data-testid="gpustack-connected"
            >
              <CircleCheck className="size-4" />
              {t('gpustack.connected', { count: discovered.length })}
            </div>
            <ModelGroups
              models={notAdded}
              selected={selectedNames}
              onToggle={toggle}
              onKindChange={setKind}
              t={t}
            />
            <p className="text-xs text-text-secondary">
              {t('gpustack.typeHint')}
            </p>
            <div>
              <Button
                onClick={addConnection}
                disabled={
                  saving ||
                  discovering ||
                  selectedModels.length === 0 ||
                  !nameValid
                }
                data-testid="gpustack-add-models"
              >
                {saving && <Loader2 className="size-4 animate-spin" />}
                {saving
                  ? t('gpustack.adding')
                  : t('gpustack.addModels', { count: selectedModels.length })}
              </Button>
            </div>
          </>
        )}
      </div>
    );
  }

  return (
    <div
      className="rounded-lg bg-bg-card p-5 mb-5 flex flex-col gap-4"
      data-testid={`instance-card-${instance.instance_name}`}
    >
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="static"
          size="auto"
          className="min-w-0 flex-1 shrink justify-start text-left"
          onClick={toggleOpen}
          aria-expanded={open}
        >
          {open ? (
            <ChevronDown className="size-4 shrink-0" />
          ) : (
            <ChevronRight className="size-4 shrink-0" />
          )}
          <span className="truncate text-sm font-medium text-text-primary">
            {instance.instance_name}
          </span>
          {open && savedBaseUrl && (
            <span className="truncate text-xs text-text-secondary">
              {savedBaseUrl}
            </span>
          )}
          {discovered && (
            <span className="shrink-0 text-xs text-state-success">
              {t('gpustack.connectedShort', { count: discovered.length })}
            </span>
          )}
        </Button>
        {open && (
          <>
            <Button
              variant="outline"
              size="sm"
              onClick={discover}
              disabled={discovering}
              data-testid="gpustack-refresh"
            >
              <RefreshCw
                className={discovering ? 'size-4 animate-spin' : 'size-4'}
              />
              {t('gpustack.refresh')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={toggleEditing}
              data-testid="gpustack-edit"
            >
              {t('gpustack.editInstance')}
            </Button>
          </>
        )}
        <ConfirmDeleteDialog onOk={handleDelete}>
          <Button
            variant="delete"
            size="icon-sm"
            aria-label={t('deleteInstance')}
          >
            <Trash2 className="size-4" />
          </Button>
        </ConfirmDeleteDialog>
      </div>

      {open && (
        <>
          {editing && (
            <div className="flex flex-col gap-3">
              {connectionForm}
              {!instanceModelsLoaded && !instanceModelsLoading && (
                <p
                  role="alert"
                  className="text-sm text-state-error"
                  data-testid="gpustack-models-load-failed"
                >
                  {t('gpustack.modelsLoadFailed')}
                </p>
              )}
              <div>
                <Button
                  onClick={saveConnection}
                  disabled={saving || !baseUrl.trim() || !instanceModelsReady}
                  data-testid="gpustack-save-connection"
                >
                  {t('gpustack.saveInstance')}
                </Button>
              </div>
            </div>
          )}
          {error && <ConnectionError message={error} t={t} />}
          {discovered && discovered.length === 0 && added.length === 0 && (
            <EmptyModels t={t} />
          )}
          {notAdded.length > 0 && (
            <div className="flex flex-col gap-3">
              <h3 className="text-sm font-medium text-text-primary">
                {t('gpustack.availableTitle')} ({notAdded.length})
              </h3>
              <ModelGroups
                models={notAdded}
                selected={selectedNames}
                onToggle={toggle}
                onKindChange={setKind}
                t={t}
              />
              <div>
                <Button
                  onClick={addToSaved}
                  disabled={saving || selectedModels.length === 0}
                  data-testid="gpustack-add-selected"
                >
                  {saving
                    ? t('gpustack.adding')
                    : t('gpustack.addSelected', {
                        count: selectedModels.length,
                      })}
                </Button>
              </div>
            </div>
          )}
          <div className="flex flex-col gap-3">
            <h3 className="text-sm font-medium text-text-primary">
              {t('gpustack.addedTitle')} ({added.length})
            </h3>
            {added.length === 0 ? (
              <p className="text-sm text-text-secondary">
                {t('gpustack.noAdded')}
              </p>
            ) : (
              <ModelGroups
                models={
                  discovered
                    ? added
                    : added.map((model) => ({ ...model, running: undefined }))
                }
                onKindChange={changeSavedKind}
                onRemove={removeModel}
                t={t}
              />
            )}
          </div>
        </>
      )}
    </div>
  );
});
