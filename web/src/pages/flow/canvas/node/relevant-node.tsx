import { Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { RightHandleStyle } from './handle-icon';
import NodePopover from './popover';

import { get } from 'lodash';
import styles from './index.less';
import NodeHeader from './node-header';

export function RelevantNode({ id, data, selected }: NodeProps<NodeData>) {
  const yes = get(data, 'form.yes');
  const no = get(data, 'form.no');
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
        <Handle
          type="source"
          position={Position.Right}
          isConnectable
          className={styles.handle}
          id={'yes'}
          style={{ ...RightHandleStyle, top: 59 }}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable
          className={styles.handle}
          id={'no'}
          style={{ ...RightHandleStyle, top: 112 }}
        ></Handle>
        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={styles.nodeHeader}
        ></NodeHeader>

        <Flex vertical gap={10}>
          <Flex vertical>
            <div className={styles.relevantLabel}>Yes</div>
            <div className={styles.nodeText}>{yes}</div>
          </Flex>
          <Flex vertical>
            <div className={styles.relevantLabel}>No</div>
            <div className={styles.nodeText}>{no}</div>
          </Flex>
        </Flex>
      </section>
    </NodePopover>
  );
}
