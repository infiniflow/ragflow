import { IBeginNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { NodeHandleId, Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import { NodeWrapper } from './node-wrapper';

// TODO: do not allow other nodes to connect to this node
function InnerBeginNode({ data, id, selected }: NodeProps<IBeginNode>) {
  const { t } = useTranslation();

  return (
    <NodeWrapper selected={selected}>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable
        style={RightHandleStyle}
        nodeId={id}
        id={NodeHandleId.Start}
      ></CommonHandle>

      <section className="flex items-center  gap-2">
        <OperatorIcon name={data.label as Operator}></OperatorIcon>
        <div className="truncate text-center font-semibold text-sm">
          {t(`dataflow.begin`)}
        </div>
      </section>
    </NodeWrapper>
  );
}

export const BeginNode = memo(InnerBeginNode);
