import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Operator } from '@/pages/agent/constant';
import { AgentFormContext, AgentInstanceContext } from '@/pages/agent/context';
import { Position } from '@xyflow/react';
import { PropsWithChildren, useCallback, useContext } from 'react';
import { ToolCommand } from './tool-command';

export function ToolPopover({ children }: PropsWithChildren) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const node = useContext(AgentFormContext);

  const handleChange = useCallback(
    (value: string[]) => {
      if (Array.isArray(value) && value.length > 0) {
        addCanvasNode(Operator.Tool, {
          position: Position.Bottom,
          nodeId: node?.id,
        })();
      }
    },
    [addCanvasNode, node?.id],
  );

  return (
    <Popover>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent className="w-80 p-0">
        <ToolCommand onChange={handleChange}></ToolCommand>
      </PopoverContent>
    </Popover>
  );
}
