import { Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import { RightHandleStyle } from './handle-icon';
import { useBuildCategorizeHandlePositions } from './hooks';
import styles from './index.less';
import NodePopover from './popover';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const { positions } = useBuildCategorizeHandlePositions({ data, id });

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
            color={operatorMap[data.label as Operator].color}
          ></OperatorIcon>
          <span className={styles.nodeTitle}>{data.name}</span>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <Flex vertical gap={8}>
          {positions.map((position, idx) => {
            return (
              <div key={idx}>
                <div className={styles.nodeText}>{position.text}</div>
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
