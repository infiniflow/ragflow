import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import OperateDropdown from '@/components/operate-dropdown';
import { Flex, Space } from 'antd';
import { useCallback } from 'react';
import { Operator, operatorMap } from '../../constant';
import OperatorIcon from '../../operator-icon';
import useGraphStore from '../../store';
import styles from './index.less';

export function RagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<{ label: string }>) {
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const deleteNode = useCallback(() => {
    deleteNodeById(id);
  }, [id, deleteNodeById]);

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
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <Handle type="source" position={Position.Top} id="d" isConnectable />
      <Handle
        type="source"
        position={Position.Right}
        isConnectable={isConnectable}
        className={styles.handle}
        id="b"
      >
        {/* <PlusCircleOutlined style={{ fontSize: 10 }} /> */}
      </Handle>
      <Handle type="source" position={Position.Bottom} id="a" isConnectable />
      <Flex gap={10} justify={'space-between'}>
        <Space size={6}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={12}
          ></OperatorIcon>
          <span>{data.label}</span>
        </Space>
        <OperateDropdown
          iconFontSize={14}
          deleteItem={deleteNode}
        ></OperateDropdown>
      </Flex>
      <div className={styles.description}>
        {operatorMap[data.label as Operator].description}
      </div>
    </section>
  );
}
