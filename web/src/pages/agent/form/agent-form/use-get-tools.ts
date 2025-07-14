import { IAgentForm } from '@/interfaces/database/agent';
import { get } from 'lodash';
import { useContext, useMemo } from 'react';
import { AgentFormContext } from '../../context';

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

  const mcpIds = useMemo(() => {
    const ids: IAgentForm['mcp'] = get(node, 'data.form.mcp', []);
    return ids.map((x) => x.mcp_id);
  }, [node]);

  return { mcpIds };
}
