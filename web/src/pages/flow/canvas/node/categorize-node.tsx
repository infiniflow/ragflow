import { Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, SwitchElseTo } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';
import { RightHandleStyle } from './handle-icon';
import { useBuildCategorizeHandlePositions } from './hooks';
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

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const { positions } = useBuildCategorizeHandlePositions({ data, id });
  const operatorName = data.label;

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

        {operatorName === Operator.Switch && (
          <CategorizeHandle top={50} right={-4} id={SwitchElseTo}>
            To
          </CategorizeHandle>
        )}
        {/* {positions.map((position) => {
          return (
            <Handle
              key={position.text}
              id={position.text}
              type="source"
              position={Position.Right}
              isConnectable
              className={styles.handle}
              style={{ ...RightHandleStyle, top: position.top }}
            ></Handle>
          );
        })} */}
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
              <>
                <Flex vertical>
                  <Flex justify={'space-between'}>
                    <span>{position.text}</span>
                    <span>{getConditionKey(idx, positions.length)}</span>
                  </Flex>
                  <div className={styles.nodeText}>yes</div>
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
              </>
            );
          })}
        </Flex>
      </section>
    </NodePopover>
  );
}
