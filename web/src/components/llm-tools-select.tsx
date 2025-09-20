import { useTranslate } from '@/hooks/common-hooks';
import { useLlmToolsList } from '@/hooks/plugin-hooks';
import { Select, Space } from 'antd';

interface IProps {
  value?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
}

const LLMToolsSelect = ({ value, onChange, disabled }: IProps) => {
  const { t } = useTranslate('llmTools');
  const tools = useLlmToolsList();

  function wrapTranslation(text: string): string {
    if (!text) {
      return text;
    }

    if (text.startsWith('$t:')) {
      return t(text.substring(3));
    }

    return text;
  }

  const toolOptions = tools.map((t) => ({
    label: wrapTranslation(t.displayName),
    description: wrapTranslation(t.displayDescription),
    value: t.name,
    title: wrapTranslation(t.displayDescription),
  }));

  return (
    <Select
      mode="multiple"
      options={toolOptions}
      optionRender={(option) => (
        <Space size="large">
          {option.label}
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
