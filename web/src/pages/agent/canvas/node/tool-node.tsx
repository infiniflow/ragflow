import { NodeCollapsible } from '@/components/collapse';
import { IAgentForm, IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId, Operator } from '../../constant';
import { ToolCard } from '../../form/agent-form/agent-tools';
import { useFindMcpById } from '../../hooks/use-find-mcp-by-id';
import OperatorIcon from '@/components/operator-icon';
import useGraphStore from '../../store';
import { NodeWrapper } from './node-wrapper';

function InnerToolNode({
  id,
  isConnectable = true,
  selected,
}: NodeProps<IToolNode>) {
  const { edges, getNode } = useGraphStore();
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
        isConnectableStart={false}
        className="!bg-accent-primary !size-2"
        isConnectableEnd={false}
      />

      <NodeCollapsible items={[tools, mcpList]}>
        {(x, idx) => {
          if (Reflect.has(x, 'mcp_id')) {
            const mcp = x as unknown as IAgentForm['mcp'][number];

            return (
              <ToolCard
                key={mcp.mcp_id || `mcp-${idx}`}
                className="cursor-pointer"
                data-tool={mcp.mcp_id}
              >
                {findMcpById(mcp.mcp_id)?.name}
              </ToolCard>
            );
          }

          const tool = x as unknown as IAgentForm['tools'][number];
          // Code has no config form, so its card is not interactive: without
          // data-tool attributes the node click handler ignores the click.
          const isCode = tool.component_name === Operator.Code;

          return (
            <ToolCard
              key={tool.id || `tool-${idx}`}
              className={isCode ? undefined : 'cursor-pointer'}
              data-tool={isCode ? undefined : tool.component_name}
              data-tool-id={isCode ? undefined : tool.id}
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
