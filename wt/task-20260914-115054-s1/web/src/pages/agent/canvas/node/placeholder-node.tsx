import { Skeleton } from '@/components/ui/skeleton';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { CommonHandle } from './handle';
import { LeftHandleStyle } from './handle-icon';
import { NodeWrapper } from './node-wrapper';

function InnerPlaceholderNode({ id, selected }: NodeProps) {
  return (
    <NodeWrapper selected={selected}>
      <CommonHandle
        type="target"
        position={Position.Left}
        isConnectable
        style={LeftHandleStyle}
        nodeId={id}
        id={NodeHandleId.End}
      ></CommonHandle>
      <div className="space-y-2">
        <Skeleton className="h-8 w-8 rounded-full" />
        <Skeleton className="h-6 w-full" />
      </div>
    </NodeWrapper>
  );
}

export const PlaceholderNode = memo(InnerPlaceholderNode);
