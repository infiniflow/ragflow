import { IAgentForm } from '@/interfaces/database/agent';
import { get } from 'lodash';
import { useContext, useMemo } from 'react';
import { AgentFormContext } from '../../context';

export function useGetNodeTools() {
  const node = useContext(AgentFormContext);
  return get(node, 'data.form.tools', []) as IAgentForm['tools'];
}

export function useGetAgentToolNames() {
  const node = useContext(AgentFormContext);

  const toolNames = useMemo(() => {
    const tools: IAgentForm['tools'] = get(node, 'data.form.tools', []);
    return tools.map((x) => x.component_name);
  }, [node]);

  return { toolNames };
}

export function useGetAgentMCPIds() {
  const node = useContext(AgentFormContext);

  const mcp = useMemo(
    () => get(node, 'data.form.mcp', []) as IAgentForm['mcp'],
    [node],
  );

  const mcpIds = useMemo(() => mcp.map((x) => x.mcp_id), [mcp]);

  return { mcpIds, mcpList: mcp };
}
