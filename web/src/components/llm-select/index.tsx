import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/llm-hooks';
import { Popover, Select } from 'antd';
import LlmSettingItems from '../llm-setting-items';
import Txt2imgSettingItems from '../txt2img-setting-items';

interface IProps {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
  modelType?: LlmModelType;
}

const LLMSelect = ({
  id,
  value,
  onChange,
  disabled,
  modelType = LlmModelType.Chat,
}: IProps) => {
  const modelOptions = useComposeLlmOptionsByModelTypes([modelType]);

  // 动态生成配置内容
  const renderSettingsPanel = (selectedType: LlmModelType) => {
    return LlmModelType.Chat == selectedType ? (
      <div style={{ width: 400 }}>
        <LlmSettingItems
          formItemLayout={{ labelCol: { span: 10 }, wrapperCol: { span: 14 } }}
        />
      </div>
    ) : (
      <div style={{ width: 400 }}>
        <Txt2imgSettingItems
          formItemLayout={{ labelCol: { span: 10 }, wrapperCol: { span: 14 } }}
        />
      </div>
    );
  };

  return (
    <Popover
      content={
        modelType ? renderSettingsPanel(modelType as LlmModelType) : null
      }
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
