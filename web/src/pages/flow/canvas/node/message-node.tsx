import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import NodePopover from './popover';

export function MessageNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const messages: string[] = get(data, 'form.messages', []);

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
          [styles.selectedNode]: selected,
        })}
      >
        <Handle
          id="c"
          type="source"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          style={RightHandleStyle}
          id="b"
        ></Handle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={classNames({
            [styles.nodeHeader]: messages.length > 0,
          })}
        ></NodeHeader>

        <Flex vertical gap={8} className={styles.messageNodeContainer}>
          {messages.map((message, idx) => {
            return (
              <div className={styles.nodeText} key={idx}>
                {message}
              </div>
            );
          })}
        </Flex>
      </section>
    </NodePopover>
  );
}
