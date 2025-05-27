import { useFetchMcpServerList } from '@/hooks/mcp-server-setting-hooks';
import { Select, Space } from 'antd';
import { truncate } from 'lodash';

interface IProps {
  value?: string;
  onChange?: (value: string, option: any) => void;
  disabled?: boolean;
}

const LLMMcpServerSelect = ({ value, onChange, disabled }: IProps) => {
  const { data: mcpServers } = useFetchMcpServerList();

  const toolOptions = mcpServers.map(s => ({
    label: s.name,
    description: !!s.description ? truncate(s.description, { length: 30 }) : "",
    value: s.id,
    title: s.description,
  }));

  return (
    <Select
      mode="multiple"
      options={toolOptions}
      optionRender={option => (
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

export default LLMMcpServerSelect;
