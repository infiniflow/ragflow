import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Operator } from '@/pages/agent/constant';
import { AgentFormContext, AgentInstanceContext } from '@/pages/agent/context';
import useGraphStore from '@/pages/agent/store';
import { Position } from '@xyflow/react';
import { t } from 'i18next';
import { PropsWithChildren, useCallback, useContext, useEffect } from 'react';
import { useGetAgentMCPIds, useGetAgentToolNames } from '../use-get-tools';
import { MCPCommand, ToolCommand } from './tool-command';
import { useUpdateAgentNodeMCP } from './use-update-mcp';
import { useUpdateAgentNodeTools } from './use-update-tools';

enum ToolType {
  Common = 'common',
  MCP = 'mcp',
}

export function ToolPopover({ children }: PropsWithChildren) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const node = useContext(AgentFormContext);
  const { updateNodeTools } = useUpdateAgentNodeTools();
  const { toolNames } = useGetAgentToolNames();
  const deleteAgentToolNodeById = useGraphStore(
    (state) => state.deleteAgentToolNodeById,
  );
  const { mcpIds } = useGetAgentMCPIds();
  const { updateNodeMCP } = useUpdateAgentNodeMCP();

  const handleChange = useCallback(
    (value: string[]) => {
      if (Array.isArray(value) && node?.id) {
        updateNodeTools(value);
      }
    },
    [node?.id, updateNodeTools],
  );

  useEffect(() => {
    const total = toolNames.length + mcpIds.length;
    if (node?.id) {
      if (total > 0) {
        addCanvasNode(Operator.Tool, {
          position: Position.Bottom,
          nodeId: node?.id,
        })();
      } else {
        deleteAgentToolNodeById(node.id);
      }
    }
  }, [
    addCanvasNode,
    deleteAgentToolNodeById,
    mcpIds.length,
    node?.id,
    toolNames.length,
  ]);

  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="w-80 p-4">
        <Tabs defaultValue={ToolType.Common}>
          <TabsList>
            <TabsTrigger value={ToolType.Common}>
              {t('flow.builtIn')}
            </TabsTrigger>
            <TabsTrigger value={ToolType.MCP}>MCP</TabsTrigger>
          </TabsList>
          <TabsContent value={ToolType.Common}>
            <ToolCommand
              onChange={handleChange}
              value={toolNames}
            ></ToolCommand>
          </TabsContent>
          <TabsContent value={ToolType.MCP}>
            <MCPCommand value={mcpIds} onChange={updateNodeMCP}></MCPCommand>
          </TabsContent>
        </Tabs>
      </PopoverContent>
    </Popover>
  );
}
