import { Flex } from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import pick from 'lodash/pick';
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

export function RagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const isCategorize = data.label === Operator.Categorize;
  const categoryData = get(data, 'form.category_description') ?? {};
  const style = operatorMap[data.label as Operator];

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={pick(style, ['backgroundColor', 'width', 'height', 'color'])}
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
      <Flex vertical align="center" justify={'center'} gap={6}>
        <OperatorIcon
          name={data.label as Operator}
          fontSize={style['iconFontSize'] ?? 24}
        ></OperatorIcon>
        <span
          className={styles.type}
          style={{ fontSize: style.fontSize ?? 14 }}
        >
          {data.label}
        </span>
        <NodeDropdown id={id}></NodeDropdown>
      </Flex>

      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{data.name}</div>
      </section>
    </section>
  );
}
