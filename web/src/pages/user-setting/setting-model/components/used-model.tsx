import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Switch } from '@/components/ui/switch';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { ModelStatus } from '@/constants/llm';
import {
  useDeleteProviderInstance,
  useEditInstanceModel,
  useFetchAddedProviders,
  useFetchInstanceModels,
  useFetchProviderInstance,
  useFetchProviderInstances,
  useUpdateModelStatus,
} from '@/hooks/use-llm-request';
import {
  IAvailableProvider,
  IInstanceModel,
  IProviderInstance,
} from '@/interfaces/database/llm';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { cn } from '@/lib/utils';
import {
  ChevronsDown,
  ChevronsUp,
  Pencil,
  Settings,
  Trash2,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AddCustomModelDialog } from '../modal/provider-modal/components/add-custom-model-dialog';
import { useCustomModelFields } from '../modal/provider-modal/components/use-custom-model-fields';
import { mapModelKey } from './un-add-model';

export function UsedModel({
  handleAddModel,
  onEditInstance,
}: {
  handleAddModel: (factory: string) => void;
  onEditInstance?: (
    providerName: string,
    instance: IProviderInstance,
    models: IInstanceModel[],
  ) => void;
}) {
  const { t } = useTranslation();
  const { data: providerList } = useFetchAddedProviders();

  return (
    <div
      className="flex flex-col w-full gap-5 mb-4"
      data-testid="added-models-section"
    >
      <div className="text-text-primary text-2xl font-medium mb-2 mt-4">
        {t('setting.addedModels')}
      </div>
      {providerList.map((provider) => (
        <ProviderCard
          key={provider.name}
          provider={provider}
          handleAddModel={handleAddModel}
          onEditInstance={onEditInstance}
        />
      ))}
    </div>
  );
}

function ProviderCard({
  provider,
  handleAddModel,
  onEditInstance,
}: {
  provider: IAvailableProvider;
  handleAddModel: (factory: string) => void;
  onEditInstance?: (
    providerName: string,
    instance: IProviderInstance,
    models: IInstanceModel[],
  ) => void;
}) {
  const { data: instances } = useFetchProviderInstances(provider.name);
  if (!instances || instances.length <= 0) {
    return null;
  }

  return (
    <div
      className="w-full rounded-lg border border-border-button"
      data-testid="added-model-card"
      data-provider={provider.name}
    >
      {/* Provider header */}
      <div className="flex h-16 items-center p-4 text-text-secondary">
        <div className="flex items-center space-x-3">
          <LlmIcon name={provider.name} width={32} />
          <div className="font-medium text-xl text-text-primary">
            {provider.name}
          </div>
        </div>
      </div>
      {/* Instances */}
      {instances.length > 0 && (
        <div className="border-t border-border-button">
          {instances.map((instance) => (
            <InstanceRow
              key={instance.id}
              instance={instance}
              providerName={provider.name}
              handleAddModel={handleAddModel}
              onEditInstance={onEditInstance}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function InstanceRow({
  instance,
  providerName,
  // handleAddModel,
  onEditInstance,
}: {
  instance: IProviderInstance;
  providerName: string;
  handleAddModel: (factory: string) => void;
  onEditInstance?: (
    providerName: string,
    instance: IProviderInstance,
    models: IInstanceModel[],
  ) => void;
}) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const { deleteProviderInstance } = useDeleteProviderInstance();

  const handleDelete = async () => {
    await deleteProviderInstance({
      provider_name: providerName,
      instances: [instance.instance_name],
    });
  };

  return (
    <Collapsible
      open={visible}
      onOpenChange={setVisible}
      className="border-b border-border-button last:border-b-0"
    >
      <div>
        {/* Instance header */}
        <div className="flex items-center justify-between p-4">
          <span className="font-medium text-text-primary">
            {instance.instance_name}
          </span>
          <div className="flex items-center space-x-2">
            {/* <Button variant="outline" size="sm" onClick={handleApiKeyClick}>
              {t('setting.apiKey')}
            </Button> */}
            <CollapsibleTrigger asChild>
              <Button variant="outline" size="sm">
                <span>
                  {visible
                    ? t('setting.hideModels')
                    : t('setting.showMoreModels')}
                </span>
                {visible ? (
                  <ChevronsUp size={16} />
                ) : (
                  <ChevronsDown size={16} />
                )}
              </Button>
            </CollapsibleTrigger>
            <ConfirmDeleteDialog onOk={handleDelete}>
              <Button size="icon" variant="danger-hover">
                <Trash2 size={16} />
              </Button>
            </ConfirmDeleteDialog>
          </div>
        </div>

        {/* Models */}
        <CollapsibleContent>
          <InstanceModelList
            providerName={providerName}
            instanceName={instance.instance_name}
            instance={instance}
            onEditInstance={onEditInstance}
          />
        </CollapsibleContent>
      </div>
    </Collapsible>
  );
}

function InstanceModelList({
  providerName,
  instanceName,
  instance,
  onEditInstance,
}: {
  providerName: string;
  instanceName: string;
  instance: IProviderInstance;
  onEditInstance?: (
    providerName: string,
    instance: IProviderInstance,
    models: IInstanceModel[],
  ) => void;
}) {
  const { data: models } = useFetchInstanceModels(providerName, instanceName);
  // Lazily fetches the full instance details (incl. baseUrl) only when
  // the user opens the settings dialog — keeps the collapsed section
  // cheap and avoids the extra request for users who never click it.
  const { refetch: fetchInstanceDetails } = useFetchProviderInstance(
    providerName,
    instanceName,
  );

  const handleSettingsClick = useCallback(async () => {
    let details: IProviderInstance = instance;
    try {
      const ret = await fetchInstanceDetails();
      if (ret.data) {
        details = { ...instance, ...(ret.data as IProviderInstance) };
      }
    } catch {
      // Fall back to the list-level instance data if the show request
      // fails (e.g. network error) — the modal still gets a usable
      // baseline.
    }
    onEditInstance?.(providerName, details, models);
  }, [fetchInstanceDetails, instance, models, onEditInstance, providerName]);

  const modelTypes = useMemo(() => {
    const types = new Set<string>();
    models.forEach((m) => {
      if (m.model_type) {
        m.model_type.forEach((type) => types.add(type));
      }
    });
    return Array.from(types);
  }, [models]);

  return (
    <div className="px-4 pb-4">
      {/* Model type tags */}
      {modelTypes.length > 0 && (
        <div className="flex justify-between items-center">
          <div className="flex flex-wrap gap-2 mb-3">
            {modelTypes.map((type) => (
              <span
                key={type}
                className="px-2 py-1 text-xs bg-bg-card text-text-secondary rounded-md"
              >
                {mapModelKey[type.trim() as keyof typeof mapModelKey] || type}
              </span>
            ))}
          </div>
          {false && (
            <Button size="icon" variant="ghost" onClick={handleSettingsClick}>
              <Settings size={12} />
            </Button>
          )}
        </div>
      )}

      {/* Model list */}
      <div className="bg-bg-card rounded-lg max-h-96 overflow-auto scrollbar-auto">
        <ul>
          {models.map((model) => (
            <ModelListItem
              key={model.name}
              model={model}
              providerName={providerName}
              instanceName={instanceName}
            />
          ))}
        </ul>
      </div>
    </div>
  );
}

function ModelListItem({
  model,
  providerName,
  instanceName,
}: {
  model: IInstanceModel;
  providerName: string;
  instanceName: string;
}) {
  const { t } = useTranslation();
  const { updateModelStatus } = useUpdateModelStatus();
  const { editInstanceModel, loading: editLoading } = useEditInstanceModel();
  const customModelFields = useCustomModelFields();
  const [editOpen, setEditOpen] = useState(false);

  const handleStatusChange = (checked: boolean) => {
    updateModelStatus({
      provider_name: providerName,
      instance_name: instanceName,
      model_name: model.name,
      status: checked ? ModelStatus.Active : ModelStatus.Inactive,
    });
  };

  // Only show name / model_types / max_tokens; only model_types is editable.
  const editFields = useMemo(
    () =>
      customModelFields
        .filter((f) => ['name', 'model_types', 'max_tokens'].includes(f.name))
        .map((f) => ({
          ...f,
          disabled: f.name !== 'model_types',
        })),
    [customModelFields],
  );

  const editDefaultValues = useMemo(
    () => ({
      name: model.name,
      model_types: model.model_type ?? [],
      max_tokens: model.max_tokens ?? 0,
    }),
    [model],
  );

  const handleEditSubmit = useCallback(
    async (item: IProviderModelItem) => {
      await editInstanceModel({
        provider_name: providerName,
        instance_name: instanceName,
        model_name: [model.name],
        model_type: item.model_types ?? [],
      });
      setEditOpen(false);
    },
    [editInstanceModel, providerName, instanceName, model.name],
  );

  return (
    <li className="flex items-center border-b-[0.5px] border-border-button justify-between p-3 hover:bg-bg-card transition-colors last:border-b-0 group">
      <div className="flex items-center space-x-3">
        <span className="font-medium text-text-primary">{model.name}</span>
        {model.model_type.slice(0, 3).map((modelType) => (
          <span
            className="px-2 py-1 text-xs bg-bg-card text-text-secondary rounded-md"
            key={modelType}
          >
            {modelType}
          </span>
        ))}
        {model.model_type.length > 3 && (
          <RAGFlowTooltip
            tooltip={
              <div className="flex flex-col gap-1">
                {model.model_type.slice(3).map((type) => (
                  <span key={type}>{type}</span>
                ))}
              </div>
            }
          >
            <span
              className="px-2 py-1 text-xs bg-bg-card text-text-secondary rounded-md cursor-pointer"
              key="ellipsis"
            >
              ...
            </span>
          </RAGFlowTooltip>
        )}
        <Button
          size="icon"
          variant="ghost"
          className={cn('h-6 w-6 hidden', 'group-hover:flex')}
          onClick={(e: any) => {
            e.stopPropagation();
            setEditOpen(true);
          }}
          aria-label={t('setting.editCustomModelTitle')}
        >
          <Pencil size={12} />
        </Button>
      </div>
      <Switch
        checked={model.status === ModelStatus.Active}
        onCheckedChange={handleStatusChange}
      />
      <AddCustomModelDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        title={t('setting.editCustomModelTitle')}
        fields={editFields}
        defaultValues={editDefaultValues}
        onSubmit={handleEditSubmit}
        loading={editLoading}
        existingNames={[]}
      />
    </li>
  );
}
