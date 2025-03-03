import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import * as SelectPrimitive from '@radix-ui/react-select';
import { Popover as AntPopover, Select as AntSelect } from 'antd';
import { forwardRef, useState } from 'react';
import LlmSettingItems from '../llm-setting-items';
import { LlmSettingFieldItems } from '../llm-setting-items/next';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { Select, SelectTrigger, SelectValue } from '../ui/select';

interface IProps {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

const LLMSelect = ({ id, value, onChange, disabled }: IProps) => {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  const content = (
    <div style={{ width: 400 }}>
      <LlmSettingItems
        formItemLayout={{ labelCol: { span: 10 }, wrapperCol: { span: 14 } }}
      ></LlmSettingItems>
    </div>
  );

  return (
    <AntPopover
      content={content}
      trigger="click"
      placement="left"
      arrow={false}
      destroyTooltipOnHide
    >
      <AntSelect
        options={modelOptions}
        style={{ width: '100%' }}
        dropdownStyle={{ display: 'none' }}
        id={id}
        value={value}
        onChange={onChange}
        disabled={disabled}
      />
    </AntPopover>
  );
};

export default LLMSelect;

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
