import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Popover as AntPopover, Select as AntSelect } from 'antd';
import { useState } from 'react';
import LlmSettingItems from '../llm-setting-items';
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

export function NextLLMSelect({ value, onChange, disabled }: IProps) {
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);

  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <Popover open={isPopoverOpen} onOpenChange={setIsPopoverOpen}>
        <PopoverTrigger asChild>
          <SelectTrigger
            onClick={(e) => {
              e.preventDefault();
              setIsPopoverOpen(true);
            }}
          >
            <SelectValue placeholder="xxx" />
          </SelectTrigger>
        </PopoverTrigger>
        <PopoverContent side={'left'}>
          <LlmSettingItems
            formItemLayout={{
              labelCol: { span: 10 },
              wrapperCol: { span: 14 },
            }}
          ></LlmSettingItems>
        </PopoverContent>
      </Popover>
    </Select>
  );
}
