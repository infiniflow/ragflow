import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import lowerFirst from 'lodash/lowerFirst';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, SwitchElseTo, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import CategorizeHandle from './categorize-handle';
import NodeDropdown from './dropdown';
import { useBuildCategorizeHandlePositions } from './hooks';
import styles from './index.less';
import NodePopover from './popover';

export function CategorizeNode({ id, data, selected }: NodeProps<NodeData>) {
  const style = operatorMap[data.label as Operator];
  const { t } = useTranslate('flow');
  const { positions } = useBuildCategorizeHandlePositions({ data, id });
  const operatorName = data.label;

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
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
        {operatorName === Operator.Switch && (
          <CategorizeHandle top={50} right={-4} id={SwitchElseTo}>
            To
          </CategorizeHandle>
        )}
        {positions.map((position, idx) => {
          return (
            <CategorizeHandle
              top={position.top}
              right={position.right}
              key={idx}
              id={position.text}
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
