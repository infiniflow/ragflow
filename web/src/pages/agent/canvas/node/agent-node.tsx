import { useTheme } from '@/components/theme-provider';
import { IAgentNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import classNames from 'classnames';
import { memo, useMemo } from 'react';
import { Operator } from '../../constant';
import useGraphStore from '../../store';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

function InnerAgentNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<IAgentNode>) {
  const { theme } = useTheme();
  const getNode = useGraphStore((state) => state.getNode);
  const edges = useGraphStore((state) => state.edges);

  const isNotParentAgent = useMemo(() => {
    const edge = edges.find((x) => x.target === id);
    const label = getNode(edge?.source)?.data.label;
    return label !== Operator.Agent;
  }, [edges, getNode, id]);

  return (
    <section
      className={classNames(
        styles.ragNode,
        theme === 'dark' ? styles.dark : '',
        {
          [styles.selectedNode]: selected,
        },
      )}
    >
      {isNotParentAgent && (
        <>
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
        </>
      )}
      <Handle
        type="target"
        position={Position.Top}
        isConnectable={false}
        id="f"
      ></Handle>
      <Handle
        type="source"
        position={Position.Bottom}
        isConnectable={false}
        id="e"
        style={{ left: 180 }}
      ></Handle>
      <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>
    </section>
  );
}

export const AgentNode = memo(InnerAgentNode);
