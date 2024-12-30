import LLMLabel from '@/components/llm-select/llm-label';
import { useTheme } from '@/components/theme-provider';
import { ICategorizeNode } from '@/interfaces/database/flow';
import { Handle, NodeProps, Position } from '@xyflow/react';
import { Flex } from 'antd';
import classNames from 'classnames';
import { get } from 'lodash';
import { RightHandleStyle } from './handle-icon';
import { useBuildCategorizeHandlePositions } from './hooks';
import styles from './index.less';
import NodeHeader from './node-header';

export function CategorizeNode({
  id,
  data,
  selected,
}: NodeProps<ICategorizeNode>) {
  const { positions } = useBuildCategorizeHandlePositions({ data, id });
  const { theme } = useTheme();
  return (
    <section
      className={classNames(
        styles.logicNode,
        theme === 'dark' ? styles.dark : '',
        {
          [styles.selectedNode]: selected,
        },
      )}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable
        className={styles.handle}
        id={'a'}
      ></Handle>

      <NodeHeader
        id={id}
        name={data.name}
        label={data.label}
        className={styles.nodeHeader}
      ></NodeHeader>

      <Flex vertical gap={8}>
        <div className={styles.nodeText}>
          <LLMLabel value={get(data, 'form.llm_id')}></LLMLabel>
        </div>
        {positions.map((position, idx) => {
          return (
            <div key={idx}>
              <div className={styles.nodeText}>{position.text}</div>
              <Handle
                key={position.text}
                id={position.text}
                type="source"
                position={Position.Right}
                isConnectable
                className={styles.handle}
                style={{ ...RightHandleStyle, top: position.top }}
              ></Handle>
            </div>
          );
        })}
      </Flex>
    </section>
  );
}
