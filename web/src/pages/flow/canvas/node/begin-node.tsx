import classNames from 'classnames';
import { useTranslation } from 'react-i18next';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { RightHandleStyle } from './handle-icon';
import styles from './index.less';

// TODO: do not allow other nodes to connect to this node
export function BeginNode({ selected }: NodeProps<NodeData>) {
  const { t } = useTranslation();

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={{
        width: 80,
      }}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
        style={RightHandleStyle}
      ></Handle>

      <div className={styles.nodeTitle}>{t(`flow.begin`)}</div>
    </section>
  );
}
