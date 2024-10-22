import { Divider, Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator } from '../../constant';
import { ISwitchCondition, NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import { RightHandleStyle } from './handle-icon';
import { useBuildSwitchHandlePositions } from './hooks';
import styles from './index.less';
import NodePopover from './popover';

const getConditionKey = (idx: number, length: number) => {
  if (idx === 0) {
    return 'If';
  } else if (idx === length - 1) {
    return 'Else';
  }

  return 'ElseIf';
};

const ConditionBlock = ({ condition }: { condition: ISwitchCondition }) => {
  const items = condition?.items ?? [];
  return (
    <Flex vertical className={styles.conditionBlock}>
      {items.map((x, idx) => (
        <div key={idx}>
          <Flex>
            <div
              className={classNames(styles.conditionLine, styles.conditionKey)}
            >
              {x?.cpn_id}
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

export function SwitchNode({ id, data, selected }: NodeProps<NodeData>) {
  const { positions } = useBuildSwitchHandlePositions({ data, id });

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
          [styles.selectedNode]: selected,
        })}
      >
        <Handle
          type="target"
          position={Position.Left}
          isConnectable
          className={styles.handle}
          id={'a'}
        ></Handle>

        <Flex
          align="center"
          justify={'space-between'}
          gap={6}
          flex={1}
          className={styles.nodeHeader}
        >
          <OperatorIcon
            name={data.label as Operator}
            fontSize={24}
          ></OperatorIcon>
          <span className={styles.nodeTitle}>{data.name}</span>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <Flex vertical gap={10}>
          {positions.map((position, idx) => {
            return (
              <div key={idx}>
                <Flex vertical>
                  <Flex justify={'space-between'}>
                    <span>{position.text}</span>
                    <span>{getConditionKey(idx, positions.length)}</span>
                  </Flex>
                  {position.condition && (
                    <ConditionBlock
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
    </NodePopover>
  );
}
