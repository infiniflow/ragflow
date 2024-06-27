import { Flex, Space } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';

import styles from './index.less';

// TODO: do not allow other nodes to connect to this node
export function BeginNode({ id, data, selected }: NodeProps<NodeData>) {
  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="source"
        position={Position.Right}
        isConnectable
        className={styles.handle}
      ></Handle>
      <Flex vertical align="center" justify="center">
        <Space size={6}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={16}
          ></OperatorIcon>
          <NodeDropdown id={id}></NodeDropdown>
        </Space>
      </Flex>
      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{id}</div>
      </section>
    </section>
  );
}
