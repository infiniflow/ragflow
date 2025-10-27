import { getLLMIconName, getLlmNameAndFIdByLlmId } from '@/utils/llm-util';
import { memo } from 'react';
import { LlmIcon } from '../svg-icon';

interface IProps {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

const LLMLabel = ({ value }: IProps) => {
  const { llmName, fId } = getLlmNameAndFIdByLlmId(value);

  return (
    <div className="flex items-center gap-1 text-xs text-text-secondary">
      <LlmIcon
        name={getLLMIconName(fId, llmName)}
        width={20}
        height={20}
        size={'small'}
      />
      <span className="flex-1 truncate"> {llmName}</span>
    </div>
  );
};

export default memo(LLMLabel);
