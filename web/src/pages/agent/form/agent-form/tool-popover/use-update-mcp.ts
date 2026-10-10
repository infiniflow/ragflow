import { useListMcpServer } from '@/hooks/use-mcp-request';
import { IAgentForm } from '@/interfaces/database/agent';
import { AgentFormContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { get } from 'lodash';
import { useCallback, useContext, useMemo } from 'react';

export function useGetNodeMCP() {
  const node = useContext(AgentFormContext);

  return useMemo(() => {
    const mcp: IAgentForm['mcp'] = get(node, 'data.form.mcp');
    return mcp;
  }, [node]);
}

export function useUpdateAgentNodeMCP() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const node = useContext(AgentFormContext);
  const mcpList = useGetNodeMCP();
  const { data } = useListMcpServer();
  const mcpServers = data.mcp_servers;

  const updateNodeMCP = useCallback(
    (value: string[]) => {
      if (node?.id) {
        const nextValue = value.reduce<IAgentForm['mcp']>((pre, cur) => {
          const mcp = mcpList.find((x) => x.mcp_id === cur);
          const mcpServer = mcpServers.find((x) => x.id === cur);
          const tools = mcpServer?.variables.tools;
          if (mcp) {
            pre.push({
              ...mcp,
              name: mcp.name ?? mcpServer?.name,
            });
          } else if (tools) {
            pre.push({
              mcp_id: cur,
              name: mcpServer?.name,
              tools: {},
            });
          }
          return pre;
        }, []);

        updateNodeForm(node?.id, nextValue, ['mcp']);
      }
    },
    [node?.id, updateNodeForm, mcpList, mcpServers],
  );

  return { updateNodeMCP };
}

export function useDeleteAgentNodeMCP() {
  const { updateNodeForm } = useGraphStore((state) => state);
  const mcpList = useGetNodeMCP();
  const node = useContext(AgentFormContext);

  const deleteNodeMCP = useCallback(
    (value: string) => () => {
      const nextMCP = mcpList.filter((x) => x.mcp_id !== value);
      if (node?.id) {
        updateNodeForm(node?.id, nextMCP, ['mcp']);
      }
    },
    [node?.id, mcpList, updateNodeForm],
  );

  return { deleteNodeMCP };
}
