import { Flex } from 'antd';
import classNames from 'classnames';
import pick from 'lodash/pick';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';

import CategorizeHandle from './categorize-handle';
import styles from './index.less';

export function RelevantNode({ id, data, selected }: NodeProps<NodeData>) {
  const style = operatorMap[data.label as Operator];

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
      style={pick(style, ['backgroundColor', 'width', 'height', 'color'])}
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
      <CategorizeHandle top={20} right={6} text={'yes'}></CategorizeHandle>
      <CategorizeHandle top={80} right={6} text={'no'}></CategorizeHandle>
      <Flex vertical align="center" justify="center">
        <OperatorIcon
          name={data.label as Operator}
          fontSize={style.iconFontSize}
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
