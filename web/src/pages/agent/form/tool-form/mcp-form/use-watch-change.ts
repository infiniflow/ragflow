import { useGetMcpServer } from '@/hooks/use-mcp-request';
import useGraphStore from '@/pages/agent/store';
import { getAgentNodeMCP } from '@/pages/agent/utils';
import { pick } from 'lodash';
import { useEffect, useMemo } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

export function useWatchFormChange(form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });
  const { clickedToolId, clickedNodeId, findUpstreamNodeById, updateNodeForm } =
    useGraphStore((state) => state);
  const { data } = useGetMcpServer(clickedToolId);

  const nextMCPTools = useMemo(() => {
    const mcpTools = data.variables?.tools || [];
    values = form?.getValues();

    return pick(mcpTools, values.items);
  }, [values, data?.variables]);

  useEffect(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    // Manually triggered form updates are synchronized to the canvas
    if (agentNode) {
      const agentNodeId = agentNode?.id;
      const mcpList = getAgentNodeMCP(agentNode);

      const nextMCP = mcpList.map((x) => {
        if (x.mcp_id === clickedToolId) {
          return {
            ...x,
            tools: nextMCPTools,
          };
        }
        return x;
      });

      updateNodeForm(agentNodeId, nextMCP, ['mcp']);
    }
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    nextMCPTools,
    updateNodeForm,
  ]);
}
