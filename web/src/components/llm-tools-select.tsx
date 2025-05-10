import { useLlmToolsList } from '@/hooks/plugin-hooks';
import { Select, Space } from 'antd';

interface IProps {
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

const LLMToolsSelect = ({ value, onChange, disabled }: IProps) => {
  const tools = useLlmToolsList();

  const toolOptions = tools.map(t => ({
    description: t.description,
    value: t.name,
    title: t.description,
  }));

  return (
    <Select
      mode="multiple"
      options={toolOptions}
      optionRender={option => (
        <Space>
          {option.value}
          {option.data.description}
        </Space>
      )}
      onChange={onChange}
      value={value}
      disabled={disabled}
    ></Select>
  );
};

export default LLMToolsSelect;
