import { Flex } from 'antd';
import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
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
      >
        <Handle
          id="c"
          type="source"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={LeftHandleStyle}
        ></Handle>
        <Handle
          type="source"
          position={Position.Right}
          isConnectable={isConnectable}
          className={styles.handle}
          id="b"
          style={RightHandleStyle}
        ></Handle>
        <Flex align="center" justify={'space-between'}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={style?.iconFontSize ?? 16}
            width={style?.iconWidth}
            color={operatorMap[data.label as Operator].color}
          ></OperatorIcon>
          <div className={styles.nodeTitle}>{data.name}</div>
          <NodeDropdown
            id={id}
            iconFontColor={style?.moreIconColor}
          ></NodeDropdown>
        </Flex>
      </section>
    </NodePopover>
  );
}
