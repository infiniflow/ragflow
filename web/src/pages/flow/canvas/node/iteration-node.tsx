import { useTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import { CirclePower } from 'lucide-react';
import { Handle, NodeProps, NodeResizeControl, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

function ResizeIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="20"
      height="20"
      viewBox="0 0 24 24"
      strokeWidth="2"
      stroke="#ff0071"
      fill="none"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{ position: 'absolute', right: 5, bottom: 5 }}
    >
      <path stroke="none" d="M0 0h24v24H0z" fill="none" />
      <polyline points="16 20 20 20 20 16" />
      <line x1="14" y1="14" x2="20" y2="20" />
      <polyline points="8 4 4 4 4 8" />
      <line x1="4" y1="4" x2="10" y2="10" />
    </svg>
  );
}

const controlStyle = {
  background: 'transparent',
  border: 'none',
};

export function IterationNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { theme } = useTheme();

  return (
    <section
      className={cn('w-full h-full', styles.iterationNode, {
        [styles.dark]: theme === 'dark',
        [styles.selectedNode]: selected,
      })}
    >
      <NodeResizeControl style={controlStyle} minWidth={100} minHeight={50}>
        <ResizeIcon />
      </NodeResizeControl>
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
      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        wrapperClassName="p-2 bg-white rounded-t-[10px]"
      ></NodeHeader>
    </section>
  );
}

export function IterationStartNode({
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { theme } = useTheme();

  return (
    <section
      className={cn('bg-white p-2 rounded-xl', {
        [styles.dark]: theme === 'dark',
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
        style={RightHandleStyle}
      ></Handle>
      <div>
        <CirclePower className="size-7" />
      </div>
    </section>
  );
}
