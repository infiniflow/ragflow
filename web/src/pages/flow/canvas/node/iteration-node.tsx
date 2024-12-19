import { useTheme } from '@/components/theme-provider';
import { cn } from '@/lib/utils';
import { CirclePower } from 'lucide-react';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import useGraphStore from '../../store';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

export function IterationNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { theme } = useTheme();
  const subNodes = useGraphStore((store) =>
    store.nodes.filter((n) => n.parentId === id),
  );
  return (
    <section
      className={cn('bg-white w-full h-full', {
        [styles.dark]: theme === 'dark',
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
        id="b"
        style={RightHandleStyle}
      ></Handle>
      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.iterationNodeHeader}
      ></NodeHeader>
      <div>xxx</div>
      {subNodes.map((subNode) => {
        return (
          <div key={subNode.id} className="bg-amber-100">
            {subNode.data.label}
          </div>
        );
      })}
    </section>
  );
}

export function IterationStartNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { theme } = useTheme();

  return (
    <section
      className={cn('bg-white ', {
        [styles.dark]: theme === 'dark',
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
        id="b"
        style={RightHandleStyle}
      ></Handle>
      <div>
        <CirclePower />
      </div>
    </section>
  );
}
