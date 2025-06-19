import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Operator } from '@/pages/agent/constant';
import { AgentFormContext, AgentInstanceContext } from '@/pages/agent/context';
import { Position } from '@xyflow/react';
import { PropsWithChildren, useCallback, useContext } from 'react';
import { useDeleteToolNode } from '../use-delete-tool-node';
import { useGetAgentToolNames } from '../use-get-tools';
import { ToolCommand } from './tool-command';
import { useUpdateAgentNodeTools } from './use-update-tools';

export function ToolPopover({ children }: PropsWithChildren) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const node = useContext(AgentFormContext);
  const { updateNodeTools } = useUpdateAgentNodeTools();
  const { toolNames } = useGetAgentToolNames();
  const { deleteToolNode } = useDeleteToolNode();

  const handleChange = useCallback(
    (value: string[]) => {
      if (Array.isArray(value) && node?.id) {
        updateNodeTools(value);
        if (value.length > 0) {
          addCanvasNode(Operator.Tool, {
            position: Position.Bottom,
            nodeId: node?.id,
          })();
        } else {
          deleteToolNode(node.id); // TODO: The tool node should be derived from the agent tools data
        }
      }
    },
    [addCanvasNode, deleteToolNode, node?.id, updateNodeTools],
  );

  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="w-80 p-0">
        <ToolCommand onChange={handleChange} value={toolNames}></ToolCommand>
      </PopoverContent>
    </Popover>
  );
}
