import { IBeginNode } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { NodeProps, Position } from '@xyflow/react';
import get from 'lodash/get';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BeginQueryType,
  BeginQueryTypeIconMap,
  NodeHandleId,
  Operator,
} from '../../constant';
import { BeginQuery } from '../../interface';
import OperatorIcon from '../../operator-icon';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import styles from './index.less';
import { NodeWrapper } from './node-wrapper';

// TODO: do not allow other nodes to connect to this node
function InnerFileNode({ data, id, selected }: NodeProps<IBeginNode>) {
  const { t } = useTranslation();
  const inputs: Record<string, BeginQuery> = get(data, 'form.inputs', {});

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
          {t(`flow.begin`)}
        </div>
      </section>
      <section className={cn(styles.generateParameters, 'flex gap-2 flex-col')}>
        {Object.entries(inputs).map(([key, val], idx) => {
          const Icon = BeginQueryTypeIconMap[val.type as BeginQueryType];
          return (
            <div
              key={idx}
              className={cn(styles.conditionBlock, 'flex gap-1.5 items-center')}
            >
              <Icon className="size-4" />
              <label htmlFor="">{key}</label>
              <span className={styles.parameterValue}>{val.name}</span>
              <span className="flex-1">{val.optional ? 'Yes' : 'No'}</span>
            </div>
          );
        })}
      </section>
    </NodeWrapper>
  );
}

export const FileNode = memo(InnerFileNode);
