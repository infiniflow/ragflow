import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Popover, Select } from 'antd';
import LlmSettingItems from '../llm-setting-items';

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
    <Popover
      content={content}
      trigger="click"
      placement="left"
      arrow={false}
      destroyTooltipOnHide
    >
      <Select
        options={modelOptions}
        style={{ width: '100%' }}
        dropdownStyle={{ display: 'none' }}
        id={id}
        value={value}
        onChange={onChange}
        disabled={disabled}
      />
    </Popover>
  );
};

export default LLMSelect;
