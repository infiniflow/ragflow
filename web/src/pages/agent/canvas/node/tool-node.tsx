import { NodeCollapsible } from '@/components/collapse';
import { IAgentForm, IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo } from 'react';
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
  const { edges, getNode, setClickedToolId } = useGraphStore();
  const upstreamAgentNodeId = edges.find((x) => x.target === id)?.source;
  const upstreamAgentNode = getNode(upstreamAgentNodeId);
  const { findMcpById } = useFindMcpById();

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
    <NodeWrapper selected={selected} id={id}>
      <Handle
        id={NodeHandleId.End}
        type="target"
        position={Position.Top}
        isConnectable={isConnectable}
        className="!bg-accent-primary !size-2"
      />

      <NodeCollapsible items={[tools, mcpList]}>
        {(x) => {
          if (Reflect.has(x, 'mcp_id')) {
            const mcp = x as unknown as IAgentForm['mcp'][number];

            return (
              <ToolCard
                key={mcp.mcp_id}
                onClick={(e) => {
                  if (mcp.mcp_id === Operator.Code) {
                    e.preventDefault();
                    e.stopPropagation();
                  }
                }}
                className="cursor-pointer"
                data-tool={mcp.mcp_id}
              >
                {findMcpById(mcp.mcp_id)?.name}
              </ToolCard>
            );
          }

          const tool = x as unknown as IAgentForm['tools'][number];

          return (
            <ToolCard
              key={tool.id}
              onClick={(e) => {
                if (tool.component_name === Operator.Code) {
                  e.preventDefault();
                  e.stopPropagation();
                }

                setClickedToolId(tool.id || tool.component_name);
              }}
              className="cursor-pointer"
              data-tool={tool.component_name}
              data-tool-id={tool.id}
            >
              <div className="flex gap-1 items-center pointer-events-none">
                <OperatorIcon name={tool.component_name as Operator} />

                {tool.component_name === Operator.Retrieval
                  ? tool.name
                  : tool.component_name}
              </div>
            </ToolCard>
          );
        }}
      </NodeCollapsible>
    </NodeWrapper>
  );
}

export const ToolNode = memo(InnerToolNode);
