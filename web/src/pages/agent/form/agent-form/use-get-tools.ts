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
