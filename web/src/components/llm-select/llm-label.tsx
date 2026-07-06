import { parseModelValue } from '@/utils/llm-util';
import { memo } from 'react';
import { LlmIcon } from '../svg-icon';

interface IProps {
  value?: string;
}

export const LLMLabel = ({ value }: IProps) => {
  const parsed = value ? parseModelValue(value) : null;
  const modelName = parsed?.model_name;
  const instanceName = parsed?.model_instance;
  const iconName = parsed ? parsed.model_provider : '';

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
        <span className="text-slate-400 truncate flex-shrink-0">
          {instanceName}
        </span>
      )}
    </div>
  );
};

export default memo(LLMLabel);
