import { NodeCollapsible } from '@/components/collapse';
import { IAgentForm, IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { MouseEventHandler, memo, useCallback } from 'react';
import { NodeHandleId, Operator } from '../../constant';
import { ToolCard } from '../../form/agent-form/agent-tools';
import { useFindMcpById } from '../../hooks/use-find-mcp-by-id';
import OperatorIcon from '../../operator-icon';
import useGraphStore from '../../store';
import { NodeWrapper } from './node-wrapper';

function InnerToolNode({
  id,
  isConnectable = true,
  selected,
}: NodeProps<IToolNode>) {
  const { edges, getNode } = useGraphStore((state) => state);
  const upstreamAgentNodeId = edges.find((x) => x.target === id)?.source;
  const upstreamAgentNode = getNode(upstreamAgentNodeId);
  const { findMcpById } = useFindMcpById();

  const handleClick = useCallback(
    (operator: string): MouseEventHandler<HTMLLIElement> =>
      (e) => {
        if (operator === Operator.Code) {
          e.preventDefault();
          e.stopPropagation();
        }
      },
    [],
  );

  const tools: IAgentForm['tools'] = get(
    upstreamAgentNode,
    'data.form.tools',
    [],
  );

  const mcpList: IAgentForm['mcp'] = get(
    upstreamAgentNode,
    'data.form.mcp',
    [],
  );

  return (
    <NodeWrapper selected={selected}>
      <Handle
        id={NodeHandleId.End}
        type="target"
        position={Position.Top}
        isConnectable={isConnectable}
        className="!bg-accent-primary !size-2"
      ></Handle>
      <NodeCollapsible items={[tools, mcpList]}>
        {(x) => {
          if ('mcp_id' in x) {
            const mcp = x as unknown as IAgentForm['mcp'][number];
            return (
              <ToolCard
                key={mcp.mcp_id}
                onClick={handleClick(mcp.mcp_id)}
                className="cursor-pointer"
                data-tool={x.mcp_id}
              >
                {findMcpById(mcp.mcp_id)?.name}
              </ToolCard>
            );
          }

          const tool = x as unknown as IAgentForm['tools'][number];
          return (
            <ToolCard
              key={tool.component_name}
              onClick={handleClick(tool.component_name)}
              className="cursor-pointer"
              data-tool={tool.component_name}
            >
              <div className="flex gap-1 items-center pointer-events-none">
                <OperatorIcon
                  name={tool.component_name as Operator}
                ></OperatorIcon>
                {tool.component_name}
              </div>
            </ToolCard>
          );
        }}
      </NodeCollapsible>
    </NodeWrapper>
  );
}

export const ToolNode = memo(InnerToolNode);
