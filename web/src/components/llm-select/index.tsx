import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Popover as AntPopover, Select as AntSelect } from 'antd';
import LlmSettingItems from '../llm-setting-items';

interface IProps {
  id?: string;
  value?: string;
  onInitialValue?: (value: string, option: any) => void;
  onChange?: (value: string, option: any) => void;
  disabled?: boolean;
}

const LLMSelect = ({
  id,
  value,
  onInitialValue,
  onChange,
  disabled,
}: IProps) => {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  if (onInitialValue && value) {
    for (const modelOption of modelOptions) {
      for (const option of modelOption.options) {
        if (option.value === value) {
          onInitialValue(value, option);
          break;
        }
      }
    }
  }

  const content = (
    <div style={{ width: 400 }}>
      <LlmSettingItems
        onChange={onChange}
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
