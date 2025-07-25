import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import * as SelectPrimitive from '@radix-ui/react-select';
import { forwardRef, memo, useState } from 'react';
import { LlmSettingFieldItems } from '../llm-setting-items/next';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Select, SelectTrigger, SelectValue } from '../ui/select';

interface IProps {
  id?: string;
  value?: string;
  onInitialValue?: (value: string, option: any) => void;
  onChange?: (value: string) => void;
  disabled?: boolean;
  filter?: string;
}

const NextInnerLLMSelect = forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  IProps
>(({ value, disabled, filter }, ref) => {
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);
  const modelTypes =
    filter === 'all' || filter === undefined
      ? [LlmModelType.Chat, LlmModelType.Image2text]
      : [filter as LlmModelType];
  const modelOptions = useComposeLlmOptionsByModelTypes(modelTypes);

  return (
    <Select disabled={disabled} value={value}>
      <Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
        <PopoverTrigger asChild>
          <SelectTrigger
            onClick={(e) => {
              e.preventDefault();
              setIsPopoverOpen(true);
            }}
            ref={ref}
          >
            <SelectValue>
              {
                modelOptions
                  .flatMap((x) => x.options)
                  .find((x) => x.value === value)?.label
              }
            </SelectValue>
          </SelectTrigger>
        </PopoverTrigger>
        <PopoverContent side={'left'}>
          <LlmSettingFieldItems options={modelOptions}></LlmSettingFieldItems>
        </PopoverContent>
      </Popover>
    </Select>
  );
});

NextInnerLLMSelect.displayName = 'LLMSelect';

export const NextLLMSelect = memo(NextInnerLLMSelect);
