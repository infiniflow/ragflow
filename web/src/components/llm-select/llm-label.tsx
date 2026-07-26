import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { parseModelValue } from '@/utils/llm-util';
import { memo } from 'react';
import { LlmIcon } from '../svg-icon';

interface IProps {
  value?: string;
  ownerTenantId?: string;
}

export const LLMLabel = ({ value, ownerTenantId }: IProps) => {
  const { data: models } = useFetchAllAddedModels(undefined, ownerTenantId);

  const parsed = value ? parseModelValue(value) : null;

  let modelName = parsed?.model_name;
  let instanceName = parsed?.model_instance;
  let iconName = parsed ? parsed.model_provider : '';

  if (!modelName && value) {
    const model = models.find((m) => m.model_id === value);
    if (model) {
      modelName = model.name;
      instanceName = model.instance_name;
      iconName = model.provider_name;
    }
  }

  if (!modelName) return null;

  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <LlmIcon
        name={iconName}
        width={22}
        height={22}
        imgClass="size-[22px] flex-shrink-0"
      />
      <span className="font-medium truncate">{modelName}</span>
      {instanceName && (
        <span className="text-text-secondary truncate flex-shrink-0">
          {instanceName}
        </span>
      )}
    </div>
  );
};

export default memo(LLMLabel);
