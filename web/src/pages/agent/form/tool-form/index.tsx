import { memo } from 'react';
import useGraphStore from '../../store';
import { ToolFormConfigMap } from './constant';
import MCPForm from './mcp-form';

const EmptyContent = () => <div></div>;

function ToolForm() {
  const clickedToolId = useGraphStore((state) => state.clickedToolId);
  const { getAgentToolById } = useGraphStore();
  const tool = getAgentToolById(clickedToolId);

  const ToolForm =
    ToolFormConfigMap[tool?.component_name as keyof typeof ToolFormConfigMap] ??
    MCPForm ??
    EmptyContent;

  return (
    <section>
      <ToolForm key={clickedToolId}></ToolForm>
    </section>
  );
}

export default memo(ToolForm);
