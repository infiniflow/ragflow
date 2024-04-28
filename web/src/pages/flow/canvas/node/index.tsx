import { useCallback } from 'react';
import { Handle, NodeProps, Position } from 'reactflow';

import styles from './index.less';

const handleStyle = { left: 10 };

export function TextUpdaterNode({
  data,
  isConnectable = true,
}: NodeProps<{ value: number }>) {
  const onChange = useCallback((evt) => {
    console.log(evt.target.value);
  }, []);

  return (
    <div className={styles.textUpdaterNode}>
      <Handle
        type="target"
        position={Position.Top}
        isConnectable={isConnectable}
      />
      <Handle
        type="source"
        position={Position.Bottom}
        // style={handleStyle}
        isConnectable={isConnectable}
      />
      <div>
        <label htmlFor="text">Text:</label>
        <input id="text" name="text" onChange={onChange} className="nodrag" />
      </div>
      {/* <Handle
        type="source"
        position={Position.Bottom}
        id="b"
        isConnectable={isConnectable}
      /> */}
    </div>
  );
}
