import { Flex } from 'antd';
import classNames from 'classnames';
import pick from 'lodash/pick';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import styles from './index.less';
import NodePopover from './popover';

export function RagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const style = operatorMap[data.label as Operator];

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.ragNode, {
          [styles.selectedNode]: selected,
        })}
        style={{
          ...pick(style, ['backgroundColor', 'color']),
        }}
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
        <Flex vertical align="center" justify={'space-around'}>
          <Flex flex={1} justify="center" align="center">
            <label htmlFor=""> </label>
          </Flex>

          <Flex flex={1}>
            <OperatorIcon
              name={data.label as Operator}
              fontSize={style?.iconFontSize ?? 16}
              width={style?.iconWidth}
            ></OperatorIcon>
          </Flex>
          <Flex flex={1}>
            <NodeDropdown
              id={id}
              iconFontColor={style?.moreIconColor}
            ></NodeDropdown>
          </Flex>
        </Flex>

        <section className={styles.bottomBox}>
          <div className={styles.nodeName}>{data.name}</div>
        </section>
      </section>
    </NodePopover>
  );
}
