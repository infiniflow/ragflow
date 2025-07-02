import { getLLMIconName, getLlmNameAndFIdByLlmId } from '@/utils/llm-util';
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
    <div className="flex items-center gap-1">
      <LlmIcon
        name={getLLMIconName(fId, llmName)}
        width={20}
        height={20}
        size={'small'}
      />
      {llmName}
    </div>
  );
};

export default LLMLabel;
