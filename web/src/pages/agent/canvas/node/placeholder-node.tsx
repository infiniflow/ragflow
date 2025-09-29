import { cn } from '@/lib/utils';
import { NodeProps, Position } from '@xyflow/react';
import { Skeleton } from 'antd';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import { NodeHandleId, Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import { CommonHandle } from './handle';
import { LeftHandleStyle } from './handle-icon';
import styles from './index.less';
import { NodeWrapper } from './node-wrapper';

function InnerPlaceholderNode({ data, id, selected }: NodeProps) {
  const { t } = useTranslation();

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
        <OperatorIcon name={data.label as Operator}></OperatorIcon>
        <div className="truncate text-center font-semibold text-sm">
          {t(`flow.placeholder`, 'Placeholder')}
        </div>
      </section>

      <section
        className={cn(styles.generateParameters, 'flex gap-2 flex-col mt-2')}
      >
        <Skeleton active paragraph={{ rows: 2 }} title={false} />
        <div className="flex gap-2">
          <Skeleton.Button active size="small" />
          <Skeleton.Button active size="small" />
        </div>
      </section>
    </NodeWrapper>
  );
}

export const PlaceholderNode = memo(InnerPlaceholderNode);
