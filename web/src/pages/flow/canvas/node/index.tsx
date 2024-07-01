import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import { Flex, Space } from 'antd';
import get from 'lodash/get';
import { CategorizeAnchorPointPositions, Operator } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';
import styles from './index.less';

export function RagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const isCategorize = data.label === Operator.Categorize;
  const categoryData = get(data, 'form.category_description') ?? {};

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        id="c"
        type="source"
        position={Position.Left}
        isConnectable={isConnectable}
        className={styles.handle}
      ></Handle>
      <Handle type="source" position={Position.Top} id="d" isConnectable />
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
        id="b"
      ></Handle>
      <Handle type="source" position={Position.Bottom} id="a" isConnectable />
      {isCategorize &&
        Object.keys(categoryData).map((x, idx) => (
          <CategorizeHandle
            top={CategorizeAnchorPointPositions[idx].top}
            right={CategorizeAnchorPointPositions[idx].right}
            key={idx}
            text={x}
            idx={idx}
          ></CategorizeHandle>
        ))}
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
        <div className={styles.nodeName}>{data.name}</div>
      </section>
    </section>
  );
}
