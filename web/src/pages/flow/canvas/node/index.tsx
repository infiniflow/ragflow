import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import OperateDropdown from '@/components/operate-dropdown';
import { CopyOutlined } from '@ant-design/icons';
import { Flex, MenuProps, Space } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Operator, operatorMap } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import useGraphStore from '../../store';
import styles from './index.less';

export function RagNode({
  id,
  data,
  isConnectable = true,
  selected,
}: NodeProps<NodeData>) {
  const { t } = useTranslation();
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const duplicateNodeById = useGraphStore((store) => store.duplicateNode);

  const deleteNode = useCallback(() => {
    deleteNodeById(id);
  }, [id, deleteNodeById]);

  const duplicateNode = useCallback(() => {
    duplicateNodeById(id);
  }, [id, duplicateNodeById]);

  const description = operatorMap[data.label as Operator].description;

  const items: MenuProps['items'] = [
    {
      key: '2',
      onClick: duplicateNode,
      label: (
        <Flex justify={'space-between'}>
          {t('common.copy')}
          <CopyOutlined />
        </Flex>
      ),
    },
  ];

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
      <Flex vertical align="center" justify="center">
        <Space size={6}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={16}
          ></OperatorIcon>
          {/* {data.label} */}
          <OperateDropdown
            iconFontSize={14}
            deleteItem={deleteNode}
            items={items}
          ></OperateDropdown>
        </Space>
        {/* <div className={styles.nodeName}>{id}</div> */}
      </Flex>
      {/* <div>
        <Text
          ellipsis={{ tooltip: description }}
          style={{ width: 130 }}
          className={styles.description}
        >
          {description}
        </Text>
      </div> */}
      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{id}</div>
      </section>
    </section>
  );
}
