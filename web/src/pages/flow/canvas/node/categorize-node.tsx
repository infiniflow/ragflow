import { Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { RightHandleStyle } from './handle-icon';
import { useBuildCategorizeHandlePositions } from './hooks';
import styles from './index.less';
import NodeHeader from './node-header';
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

        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={classNames({ [styles.nodeHeader]: positions.length })}
        ></NodeHeader>

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
