import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import styles from './index.less';

export function TextUpdaterNode({
  data,
  isConnectable = true,
  selected,
}: NodeProps<{ label: string }>) {
  return (
    <div
      className={classNames(styles.textUpdaterNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable={isConnectable}
        className={styles.handle}
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <div>{data.label}</div>
    </div>
  );
}
