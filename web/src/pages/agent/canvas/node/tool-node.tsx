import { IAgentForm, IToolNode } from '@/interfaces/database/agent';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
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
      <ul className="space-y-1">
        {tools.map((x) => (
          <li key={x.component_name}>{x.component_name}</li>
        ))}
      </ul>
    </NodeWrapper>
  );
}

export const ToolNode = memo(InnerToolNode);
