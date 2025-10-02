import { NodeProps, Position } from '@xyflow/react';
import { Skeleton } from 'antd';
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

      <section className="flex items-center gap-2">
        <Skeleton.Avatar
          active
          size={24}
          shape="square"
          style={{ backgroundColor: 'rgba(255,255,255,0.05)' }}
        />
      </section>

      <section className={'flex gap-2 flex-col'} style={{ marginTop: 10 }}>
        <Skeleton.Input active style={{ width: '100%', height: 30 }} />
      </section>
    </NodeWrapper>
  );
}

export const PlaceholderNode = memo(InnerPlaceholderNode);
