import { IAgentForm, IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo, useCallback } from 'react';
import { NodeHandleId } from '../../constant';
import { ToolCard } from '../../form/agent-form/agent-tools';
import useGraphStore from '../../store';
import { NodeWrapper } from './node-wrapper';

function InnerToolNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IToolNode>) {
  const { edges, getNode } = useGraphStore((state) => state);
  const upstreamAgentNodeId = edges.find((x) => x.target === id)?.source;
  const upstreamAgentNode = getNode(upstreamAgentNodeId);

  const handleClick = useCallback(() => {}, []);

  const tools: IAgentForm['tools'] = get(
    upstreamAgentNode,
    'data.form.tools',
    [],
  );

  return (
    <NodeWrapper>
      <Handle
        id={NodeHandleId.End}
        type="target"
        position={Position.Top}
        isConnectable={isConnectable}
      ></Handle>
      <ul className="space-y-2">
        {tools.map((x) => (
          <ToolCard
            key={x.component_name}
            onClick={handleClick}
            className="cursor-pointer"
            data-tool={x.component_name}
          >
            {x.component_name}
          </ToolCard>
        ))}
      </ul>
    </NodeWrapper>
  );
}

export const ToolNode = memo(InnerToolNode);
