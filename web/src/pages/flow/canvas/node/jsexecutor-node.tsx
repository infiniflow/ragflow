import { useTranslate } from '@/hooks/common-hooks';
import { Flex } from 'antd';
import classNames from 'classnames';
import { useState } from 'react';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

export function JSExecutorNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { t } = useTranslate('flow');
  const [showDetails, setShowDetails] = useState(false);
  const inputNames = (data.form?.input_names || []) as string[];

  return (
    <section
      className={classNames(styles.ragNode, {
        [styles.selectedNode]: selected,
      })}
    >
      {/* 动态生成输入连接点 */}
      {inputNames.map((name: string, index: number) => (
        <Handle
          key={`input-${index}`}
          id={`input-${index}`}
          type="target"
          position={Position.Left}
          isConnectable={isConnectable}
          className={styles.handle}
          style={{
            ...LeftHandleStyle,
            top: `${25 + index * 15}%`,
          }}
        />
      ))}

      {/* 输出连接点 */}
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
        style={RightHandleStyle}
        id="output"
      />

      <NodeHeader id={id} name={data.name} label={data.label} />

      <Flex vertical gap={8} className={styles.jsExecutorNodeContainer}>
        <div
          className={styles.scriptConfig}
          onClick={() => setShowDetails(!showDetails)}
        >
          <div className={styles.configItem}>
            <span className={styles.configLabel}>Inputs:</span>
            <span className={styles.configValue}>
              {inputNames.length > 0 ? inputNames.join(', ') : t('noInputs')}
            </span>
          </div>
          <div className={styles.expandIcon}>{showDetails ? '▼' : '▶'}</div>
        </div>

        {showDetails && (
          <div className={styles.scriptContent}>
            <pre>{data.form?.script || '// No script (passthrough)'}</pre>
          </div>
        )}
      </Flex>
    </section>
  );
}
