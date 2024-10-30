import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { useMemo } from 'react';

interface IProps {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

const LLMLabel = ({ value }: IProps) => {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  const label = useMemo(() => {
    for (const item of modelOptions) {
      for (const option of item.options) {
        if (option.value === value) {
          return option.label;
        }
      }
    }
  }, [modelOptions, value]);

  return <div>{label}</div>;
};

export default LLMLabel;
