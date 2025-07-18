import { IMessageNode } from '@/interfaces/database/flow';
import { NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { memo } from 'react';
import { NodeHandleId } from '../../constant';
import { CommonHandle } from './handle';
import { LeftHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import { NodeWrapper } from './node-wrapper';
import { ToolBar } from './toolbar';

function InnerMessageNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IMessageNode>) {
  const messages: string[] = get(data, 'form.messages', []);
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <NodeWrapper selected={selected}>
        <CommonHandle
          type="target"
          position={Position.Left}
          isConnectable={isConnectable}
          style={LeftHandleStyle}
          nodeId={id}
          id={NodeHandleId.End}
        ></CommonHandle>
        {/* <CommonHandle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          style={RightHandleStyle}
          id={NodeHandleId.Start}
          nodeId={id}
          isConnectableEnd={false}
        ></CommonHandle> */}
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
      </NodeWrapper>
    </ToolBar>
  );
}

export const MessageNode = memo(InnerMessageNode);
