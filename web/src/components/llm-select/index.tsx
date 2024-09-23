import { LlmModelType } from '@/constants/knowledge';
import { useSelectLlmOptionsByModelType } from '@/hooks/llm-hooks';
import { Popover, Select } from 'antd';
import LlmSettingItems from '../llm-setting-items';

interface IProps {
  id?: string;
  value?: string;
  onChange?: (value: string) => void;
}

const LLMSelect = ({ id, value, onChange }: IProps) => {
  const modelOptions = useSelectLlmOptionsByModelType();

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
        options={[
          ...modelOptions[LlmModelType.Chat],
          ...modelOptions[LlmModelType.Image2text],
        ]}
        style={{ width: '100%' }}
        dropdownStyle={{ display: 'none' }}
        id={id}
        value={value}
        onChange={onChange}
      />
    </Popover>
  );
};

export default LLMSelect;
