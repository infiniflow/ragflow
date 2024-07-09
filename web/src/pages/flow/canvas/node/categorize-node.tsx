import { useTranslate } from '@/hooks/commonHooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import get from 'lodash/get';
import lowerFirst from 'lodash/lowerFirst';
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
import NodePopover from './popover';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const categoryData = get(data, 'form.category_description') ?? {};
  const style = operatorMap[data.label as Operator];
  const { t } = useTranslate('flow');
  return (
    <NodePopover nodeId={id}>
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
          <span className={styles.type}>{t(lowerFirst(data.label))}</span>
          <NodeDropdown id={id}></NodeDropdown>
        </Flex>
        <section className={styles.bottomBox}>
          <div className={styles.nodeName}>{data.name}</div>
        </section>
      </section>
    </NodePopover>
  );
}
