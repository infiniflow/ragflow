import LLMLabel from '@/components/llm-select/llm-label';
import { LlmIcon } from '@/components/svg-icon';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { IAddedModel } from '@/interfaces/database/llm';
import { getRealModelName } from '@/utils/llm-util';
import { useCallback, useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { TreeSelect, TreeSelectNode } from './tree-select';

/** Maps form field names to their supported model types */
export const ModelTypeMap: Record<string, string[]> = {
  llm_id: ['chat', 'image2text'],
  embd_id: ['embedding'],
  img2txt_id: ['image2text'],
  asr_id: ['speech2text'],
  rerank_id: ['rerank'],
  tts_id: ['tts'],
};

export function buildModelTree(
  allModels: IAddedModel[],
  modelTypes: string[],
  renderLeafLabel?: (
    node: TreeSelectNode,
    model: IAddedModel,
  ) => React.ReactNode,
): TreeSelectNode[] {
  const filtered = allModels.filter((m) =>
    m.model_type?.some((t) => modelTypes.includes(t)),
  );

  const seenLeafIds = new Set<string>();
  const providerMap = new Map<string, Map<string, IAddedModel[]>>();

  for (const model of filtered) {
    let instances = providerMap.get(model.provider_name);
    if (!instances) {
      instances = new Map();
      providerMap.set(model.provider_name, instances);
    }
    let modelList = instances.get(model.instance_name);
    if (!modelList) {
      modelList = [];
      instances.set(model.instance_name, modelList);
    }
    modelList.push(model);
  }

  return Array.from(providerMap.entries()).map(([provider, instances]) => ({
    id: provider,
    title: provider,
    children: Array.from(instances.entries()).map(([instance, models]) => ({
      id: `${provider}||${instance}`,
      title: instance,
      children: models.reduce<TreeSelectNode[]>((acc, m) => {
        const modelName = getRealModelName(m.name);
        const id = `${modelName}@${m.instance_name}@${m.provider_name}`;
        if (seenLeafIds.has(id)) return acc;
        seenLeafIds.add(id);
        const leafNode: TreeSelectNode = {
          id,
          title: modelName,
          label: (
            <span className="flex items-center gap-1.5 truncate">
              <LlmIcon
                name={m.provider_name}
                width={22}
                height={22}
                imgClass="size-[22px] flex-shrink-0"
              />
              <span className="truncate">{modelName}</span>
            </span>
          ),
          data: {
            provider_name: m.provider_name,
            instance_name: m.instance_name,
            model_name: modelName,
          },
        };
        if (renderLeafLabel) {
          leafNode.label = renderLeafLabel(leafNode, m);
        }
        acc.push(leafNode);
        return acc;
      }, []),
    })),
  }));
}

export interface ModelTreeSelectProps {
  modelTypes?: string[];
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  showSearch?: boolean;
  allowClear?: boolean;
  className?: string;
  renderSelected?: (node: TreeSelectNode | undefined) => React.ReactNode;
  testId?: string;
}

export function ModelTreeSelect({
  modelTypes = ModelTypeMap.llm_id,
  value,
  onChange,
  disabled,
  placeholder,
  showSearch = true,
  allowClear = false,
  className,
  renderSelected,
  testId,
}: ModelTreeSelectProps) {
  const { data: allAddedModels } = useFetchAllAddedModels();

  const treeData = useMemo(
    () => buildModelTree(allAddedModels, modelTypes),
    [allAddedModels, modelTypes],
  );

  const defaultRenderSelected = useCallback(
    (node: TreeSelectNode | undefined) => {
      if (!node?.id) return null;
      return <LLMLabel value={node.id} />;
    },
    [],
  );

  return (
    <TreeSelect
      data={treeData}
      value={value}
      onChange={onChange}
      placeholder={placeholder}
      disabled={disabled}
      showSearch={showSearch}
      allowClear={allowClear}
      defaultExpandAll
      className={className}
      renderSelected={renderSelected ?? defaultRenderSelected}
      testId={testId}
    />
  );
}

export interface ModelTreeSelectFormFieldProps extends ModelTreeSelectProps {
  name?: string;
  label?: string;
  tooltip?: string;
}

export function ModelTreeSelectFormField({
  name = 'llm_id',
  label,
  tooltip,
  ...rest
}: ModelTreeSelectFormFieldProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          {label && <FormLabel tooltip={tooltip}>{label}</FormLabel>}
          <FormControl>
            <ModelTreeSelect
              {...rest}
              value={field.value}
              onChange={field.onChange}
              placeholder={rest.placeholder ?? t('common.pleaseSelect')}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
