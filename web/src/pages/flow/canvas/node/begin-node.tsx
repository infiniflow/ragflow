import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import lowerFirst from 'lodash/lowerFirst';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import styles from './index.less';

// TODO: do not allow other nodes to connect to this node
export function BeginNode({ id, data, selected }: NodeProps<NodeData>) {
  const { t } = useTranslate('flow');
  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={{
        backgroundColor: operatorMap[data.label as Operator].backgroundColor,
        color: 'white',
        width: 50,
        height: 50,
      }}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
      ></Handle>
      <Flex vertical align="center" justify="center" gap={6}>
        <span className={styles.type}>{t(lowerFirst(data.label))}</span>
      </Flex>
      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{data.name}</div>
      </section>
    </section>
  );
}
