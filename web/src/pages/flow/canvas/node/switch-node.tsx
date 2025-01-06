import { useTheme } from '@/components/theme-provider';
import { ISwitchCondition, ISwitchNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { Divider, Flex } from 'antd';
import classNames from 'classnames';
import { useGetComponentLabelByValue } from '../../hooks/use-get-begin-query';
import { RightHandleStyle } from './handle-icon';
import { useBuildSwitchHandlePositions } from './hooks';
import styles from './index.less';
import NodeHeader from './node-header';

const getConditionKey = (idx: number, length: number) => {
  if (idx === 0 && length !== 1) {
    return 'If';
  } else if (idx === length - 1) {
    return 'Else';
  }

  return 'ElseIf';
};

const ConditionBlock = ({
  condition,
  nodeId,
}: {
  condition: ISwitchCondition;
  nodeId: string;
}) => {
  const items = condition?.items ?? [];
  const getLabel = useGetComponentLabelByValue(nodeId);
  return (
    <Flex vertical className={styles.conditionBlock}>
      {items.map((x, idx) => (
        <div key={idx}>
          <Flex>
            <div
              className={classNames(styles.conditionLine, styles.conditionKey)}
            >
              {getLabel(x?.cpn_id)}
            </div>
            <span className={styles.conditionOperator}>{x?.operator}</span>
            <Flex flex={1} className={styles.conditionLine}>
              {x?.value}
            </Flex>
          </Flex>
          {idx + 1 < items.length && (
            <Divider orientationMargin="0" className={styles.zeroDivider}>
              {condition?.logical_operator}
            </Divider>
          )}
        </div>
      ))}
    </Flex>
  );
};

export function SwitchNode({ id, data, selected }: NodeProps<ISwitchNode>) {
  const { positions } = useBuildSwitchHandlePositions({ data, id });
  const { theme } = useTheme();
  return (
    <section
      className={classNames(
        styles.logicNode,
        theme === 'dark' ? styles.dark : '',
        {
          [styles.selectedNode]: selected,
        },
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable
        className={styles.handle}
        id={'a'}
      ></Handle>
      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.nodeHeader}
      ></NodeHeader>
      <Flex vertical gap={10}>
        {positions.map((position, idx) => {
          return (
            <div key={idx}>
              <Flex vertical>
                <Flex justify={'space-between'}>
                  <span>{idx < positions.length - 1 && position.text}</span>
                  <span>{getConditionKey(idx, positions.length)}</span>
                </Flex>
                {position.condition && (
                  <ConditionBlock
                    nodeId={id}
                    condition={position.condition}
                  ></ConditionBlock>
                )}
              </Flex>
              <Handle
                key={position.text}
                id={position.text}
                type="source"
                position={Position.Right}
                isConnectable
                className={styles.handle}
                style={{ ...RightHandleStyle, top: position.top }}
              ></Handle>
            </div>
          );
        })}
      </Flex>
    </section>
  );
}
