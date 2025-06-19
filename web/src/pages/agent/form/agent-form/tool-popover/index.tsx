import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { IAgentForm } from '@/interfaces/database/agent';
import { Operator } from '@/pages/agent/constant';
import { AgentFormContext, AgentInstanceContext } from '@/pages/agent/context';
import { Position } from '@xyflow/react';
import { get } from 'lodash';
import { PropsWithChildren, useCallback, useContext, useMemo } from 'react';
import { ToolCommand } from './tool-command';
import { useUpdateAgentNodeTools } from './use-update-tools';

export function ToolPopover({ children }: PropsWithChildren) {
  const { addCanvasNode } = useContext(AgentInstanceContext);
  const node = useContext(AgentFormContext);
  const { updateNodeTools } = useUpdateAgentNodeTools();

  const toolNames = useMemo(() => {
    const tools: IAgentForm['tools'] = get(node, 'data.form.tools', []);
    return tools.map((x) => x.component_name);
  }, [node]);

  const handleChange = useCallback(
    (value: string[]) => {
      if (Array.isArray(value) && value.length > 0 && node?.id) {
        updateNodeTools(value);
        addCanvasNode(Operator.Tool, {
          position: Position.Bottom,
          nodeId: node?.id,
        })();
      }
    },
    [addCanvasNode, node?.id, updateNodeTools],
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
