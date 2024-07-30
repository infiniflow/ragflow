import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import lowerFirst from 'lodash/lowerFirst';
import pick from 'lodash/pick';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';

import CategorizeHandle from './categorize-handle';
import styles from './index.less';
import NodePopover from './popover';

export function RelevantNode({ id, data, selected }: NodeProps<NodeData>) {
  const style = operatorMap[data.label as Operator];
  const { t } = useTranslate('flow');
  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
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
        <Flex vertical align="center" justify="center" gap={0}>
          <Flex flex={1}>
            <OperatorIcon
              name={data.label as Operator}
              fontSize={style.iconFontSize}
            ></OperatorIcon>
          </Flex>
          <Flex flex={1}>
            <span
              className={styles.type}
              style={{ fontSize: style.fontSize ?? 14 }}
            >
              {t(lowerFirst(data.label))}
            </span>
          </Flex>
          <Flex flex={1}>
            <NodeDropdown id={id}></NodeDropdown>
          </Flex>
        </Flex>
        <section className={styles.bottomBox}>
          <div className={styles.nodeName}>{data.name}</div>
        </section>
      </section>
    </NodePopover>
  );
}
