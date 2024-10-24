import LLMLabel from '@/components/llm-select/llm-label';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { Handle, NodeProps, Position } from 'reactflow';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import NodeDropdown from './dropdown';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodePopover from './popover';

const ZeroGapOperators = [
  Operator.RewriteQuestion,
  Operator.KeywordExtract,
  Operator.ArXiv,
];

export function GenerateNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const style = operatorMap[data.label as Operator];

  return (
    <NodePopover nodeId={id}>
      <section
        className={classNames(styles.logicNode, {
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
          style={RightHandleStyle}
          id="b"
        ></Handle>
        <Flex
          flex={1}
          align="center"
          justify={'space-between'}
          gap={ZeroGapOperators.some((x) => x === data.label) ? 0 : 6}
          className={styles.nodeHeader}
        >
          <OperatorIcon
            name={data.label as Operator}
            color={operatorMap[data.label as Operator].color}
          ></OperatorIcon>
          <span className={styles.nodeTitle}>{data.name}</span>
          <NodeDropdown
            id={id}
            iconFontColor={style?.moreIconColor}
          ></NodeDropdown>
        </Flex>

        <div className={styles.nodeText}>
          <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
        </div>
      </section>
    </NodePopover>
  );
}
