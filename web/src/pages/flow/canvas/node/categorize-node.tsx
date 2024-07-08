import { Flex } from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import { Handle, NodeProps, Position } from 'reactflow';
import {
  CategorizeAnchorPointPositions,
  Operator,
  operatorMap,
} from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';

import styles from './index.less';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const categoryData = get(data, 'form.category_description') ?? {};
  const style = operatorMap[data.label as Operator];

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={{
        backgroundColor: style.backgroundColor,
        color: style.color,
      }}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable
        className={styles.handle}
        id={'a'}
      ></Handle>
      <Handle
        type="target"
        position={Position.Top}
        isConnectable
        className={styles.handle}
        id={'b'}
      ></Handle>
      <Handle
        type="target"
        position={Position.Bottom}
        isConnectable
        className={styles.handle}
        id={'c'}
      ></Handle>
      {Object.keys(categoryData).map((x, idx) => {
        console.info(categoryData, id, data);
        return (
          <CategorizeHandle
            top={CategorizeAnchorPointPositions[idx].top}
            right={CategorizeAnchorPointPositions[idx].right}
            key={idx}
            text={x}
            idx={idx}
          ></CategorizeHandle>
        );
      })}
      <Flex vertical align="center" justify="center" gap={6}>
        <OperatorIcon
          name={data.label as Operator}
          fontSize={24}
        ></OperatorIcon>
        <span className={styles.type}>{data.label}</span>
        <NodeDropdown id={id}></NodeDropdown>
      </Flex>
      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{data.name}</div>
      </section>
    </section>
  );
}
