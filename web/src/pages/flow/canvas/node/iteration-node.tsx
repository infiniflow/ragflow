import { useTheme } from '@/components/theme-provider';
import {
  IIterationNode,
  IIterationStartNode,
} from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { Handle, NodeProps, NodeResizeControl, Position } from '@xyflow/react';
import { ListRestart } from 'lucide-react';
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
      stroke="#5025f9"
      fill="none"
      strokeLinecap="round"
      strokeLinejoin="round"
      style={{
        position: 'absolute',
        right: 5,
        bottom: 5,
      }}
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
  cursor: 'nwse-resize',
};

export function IterationNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IIterationNode>) {
  const { theme } = useTheme();

  return (
    <section
      className={cn(
        'w-full h-full bg-zinc-200 opacity-70',
        styles.iterationNode,
        {
          ['bg-gray-800']: theme === 'dark',
          [styles.selectedIterationNode]: selected,
        },
      )}
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
        wrapperClassName={cn(
          'p-2 bg-white rounded-t-[10px] absolute w-full top-[-60px] left-[-0.3px]',
          styles.iterationHeader,
          {
            [`${styles.dark} text-white`]: theme === 'dark',
            [styles.selectedHeader]: selected,
          },
        )}
      ></NodeHeader>
    </section>
  );
}

export function IterationStartNode({
  isConnectable = true,
  selected,
}: NodeProps<IIterationStartNode>) {
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
        isConnectableEnd={false}
      ></Handle>
      <div>
        <ListRestart className="size-7" />
      </div>
    </section>
  );
}
