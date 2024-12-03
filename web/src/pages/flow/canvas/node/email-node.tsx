import { Flex } from 'antd';
import classNames from 'classnames';
import { useState } from 'react';
import { Handle, NodeProps, Position } from 'reactflow';
import { NodeData } from '../../interface';
import { LeftHandleStyle, RightHandleStyle } from './handle-icon';
import styles from './index.less';
import NodeHeader from './node-header';

export function EmailNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const [showDetails, setShowDetails] = useState(false);

  return (
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
        style={RightHandleStyle}
        id="b"
      ></Handle>
      <NodeHeader id={id} name={data.name} label={data.label}></NodeHeader>

      <Flex vertical gap={8} className={styles.emailNodeContainer}>
        <div
          className={styles.emailConfig}
          onClick={() => setShowDetails(!showDetails)}
        >
          <div className={styles.configItem}>
            <span className={styles.configLabel}>SMTP:</span>
            <span className={styles.configValue}>{data.form?.smtp_server}</span>
          </div>
          <div className={styles.configItem}>
            <span className={styles.configLabel}>Port:</span>
            <span className={styles.configValue}>{data.form?.smtp_port}</span>
          </div>
          <div className={styles.configItem}>
            <span className={styles.configLabel}>From:</span>
            <span className={styles.configValue}>{data.form?.email}</span>
          </div>
        </div>
      </Flex>
    </section>
  );
}
