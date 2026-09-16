import { useGetMcpServer } from '@/hooks/use-mcp-request';
import useGraphStore from '@/pages/agent/store';
import { getAgentNodeMCP } from '@/pages/agent/utils';
import { isEqual, pick } from 'lodash';
import { useEffect, useMemo, useRef } from 'react';
import { UseFormReturn, useFormState, useWatch } from 'react-hook-form';

export function useWatchFormChange(form?: UseFormReturn<any>) {
  const values = useWatch({ control: form?.control });
  const { isDirty } = useFormState({ control: form?.control });
  const { clickedToolId, clickedNodeId, findUpstreamNodeById, updateNodeForm } =
    useGraphStore((state) => state);
  const { data, loading } = useGetMcpServer(clickedToolId);
  const editedPanel = useRef<string>();
  const panelKey = `${clickedNodeId}/${clickedToolId}`;

  const nextMCPTools = useMemo(() => {
    const mcpTools = data.variables?.tools;
    const items = values.items;
    if (
      loading ||
      data.id !== clickedToolId ||
      !mcpTools ||
      !Array.isArray(items) ||
      items.some(
        (name) => !Object.prototype.hasOwnProperty.call(mcpTools, name),
      )
    ) {
      return null;
    }
    return pick(mcpTools, items);
  }, [clickedToolId, data, loading, values]);

  useEffect(() => {
    const agentNode = findUpstreamNodeById(clickedNodeId);
    // Manually triggered form updates are synchronized to the canvas
    if (
      agentNode &&
      (isDirty || editedPanel.current === panelKey) &&
      nextMCPTools !== null
    ) {
      const agentNodeId = agentNode.id;
      const mcpList = getAgentNodeMCP(agentNode);
      const selectedMCP = mcpList.find((x) => x.mcp_id === clickedToolId);
      if (!selectedMCP || isEqual(selectedMCP.tools, nextMCPTools)) {
        return;
      }

      const nextMCP = mcpList.map((x) => {
        if (x.mcp_id === clickedToolId) {
          return {
            ...x,
            tools: nextMCPTools,
          };
        }
        return x;
      });

      editedPanel.current = panelKey;
      updateNodeForm(agentNodeId, nextMCP, ['mcp']);
    }
  }, [
    clickedNodeId,
    clickedToolId,
    findUpstreamNodeById,
    isDirty,
    nextMCPTools,
    panelKey,
    updateNodeForm,
  ]);
}
