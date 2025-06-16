import { IBeginNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import get from 'lodash/get';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  BeginQueryType,
  BeginQueryTypeIconMap,
  Operator,
} from '../../constant';
import { BeginQuery } from '../../interface';
import OperatorIcon from '../../operator-icon';
import { CommonHandle } from './handle';
import { RightHandleStyle } from './handle-icon';
import styles from './index.less';
import { NodeWrapper } from './node-wrapper';

// TODO: do not allow other nodes to connect to this node
function InnerBeginNode({ data }: NodeProps<IBeginNode>) {
  const { t } = useTranslation();
  const query: BeginQuery[] = get(data, 'form.query', []);

  return (
    <NodeWrapper>
      <CommonHandle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        style={RightHandleStyle}
      ></CommonHandle>

      <Flex align="center" justify={'center'} gap={10}>
        <OperatorIcon name={data.label as Operator}></OperatorIcon>
        <div className="truncate text-center font-semibold text-sm">
          {t(`flow.begin`)}
        </div>
      </Flex>
      <Flex gap={8} vertical className={styles.generateParameters}>
        {query.map((x, idx) => {
          const Icon = BeginQueryTypeIconMap[x.type as BeginQueryType];
          return (
            <Flex
              key={idx}
              align="center"
              gap={6}
              className={styles.conditionBlock}
            >
              <Icon className="size-4" />
              <label htmlFor="">{x.key}</label>
              <span className={styles.parameterValue}>{x.name}</span>
              <span className="flex-1">{x.optional ? 'Yes' : 'No'}</span>
            </Flex>
          );
        })}
      </Flex>
    </NodeWrapper>
  );
}

export const BeginNode = memo(InnerBeginNode);
