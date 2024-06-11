import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import { Space } from 'antd';
import { Operator } from '../../constant';
import OperatorIcon from '../../operator-icon';
import styles from './index.less';

export function TextUpdaterNode({
  data,
  isConnectable = true,
  selected,
}: NodeProps<{ label: string }>) {
  return (
    <section
      className={classNames(styles.textUpdaterNode, {
        [styles.selectedNode]: selected,
      })}
    >
      <Handle
        type="target"
        position={Position.Left}
        isConnectable={isConnectable}
        className={styles.handle}
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <div>
        <Space>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={12}
          ></OperatorIcon>
          {data.label}
        </Space>
      </div>
    </section>
  );
}
