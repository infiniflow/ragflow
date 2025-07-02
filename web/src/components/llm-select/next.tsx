import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import * as SelectPrimitive from '@radix-ui/react-select';
import { forwardRef, useState } from 'react';
import { LlmSettingFieldItems } from '../llm-setting-items/next';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Select, SelectTrigger, SelectValue } from '../ui/select';

interface IProps {
  id?: string;
  value?: string;
  onInitialValue?: (value: string, option: any) => void;
  onChange?: (value: string, option: any) => void;
  disabled?: boolean;
}

export const NextLLMSelect = forwardRef<
  React.ElementRef<typeof SelectPrimitive.Trigger>,
  IProps
>(({ value, disabled }, ref) => {
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

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
          <LlmSettingFieldItems></LlmSettingFieldItems>
        </PopoverContent>
      </Popover>
    </Select>
  );
});

NextLLMSelect.displayName = 'LLMSelect';
