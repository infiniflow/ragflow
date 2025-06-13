import { useTheme } from '@/components/theme-provider';
import { IRagNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { memo } from 'react';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import { ToolBar } from './toolbar';

function InnerRagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IRagNode>) {
  const { theme } = useTheme();
  return (
    <ToolBar selected={selected} id={id} label={data.label}>
      <section
        className={classNames(
          styles.ragNode,
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
          id="b"
          style={RightHandleStyle}
        ></Handle>
        <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
      </section>
    </ToolBar>
  );
}

export const RagNode = memo(InnerRagNode);
