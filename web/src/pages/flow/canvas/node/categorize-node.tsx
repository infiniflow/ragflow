import { Flex, Space } from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import { Handle, NodeProps, Position } from 'reactflow';
import { CategorizeAnchorPointPositions, Operator } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';

import styles from './index.less';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const categoryData = get(data, 'form.category_description') ?? {};

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable
        className={styles.handle}
      ></Handle>
      {Object.keys(categoryData).map((x, idx) => (
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
        <div className={styles.nodeName}>{id}</div>
      </section>
    </section>
  );
}
