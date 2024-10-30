import LLMLabel from '@/components/llm-select/llm-label';
import classNames from 'classnames';
import { get } from 'lodash';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';
import NodePopover from './popover';

export function KeywordNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
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

        <NodeHeader
          id={id}
          name={data.name}
          label={data.label}
          className={styles.nodeHeader}
        ></NodeHeader>

        <div className={styles.nodeText}>
          <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
        </div>
      </section>
    </NodePopover>
  );
}
