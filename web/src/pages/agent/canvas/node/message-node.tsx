import { useTheme } from '@/components/theme-provider';
import { IMessageNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

export function MessageNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IMessageNode>) {
  const messages: string[] = get(data, 'form.messages', []);
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
  );
}
