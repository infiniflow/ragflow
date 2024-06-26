import classNames from 'classnames';
import { Handle, NodeProps, Position } from 'reactflow';

import OperateDropdown from '@/components/operate-dropdown';
import { CopyOutlined } from '@ant-design/icons';
import { Flex, MenuProps, Space } from 'antd';
import get from 'lodash/get';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { CategorizeAnchorPointPositions, Operator } from '../../constant';
import { NodeData } from '../../interface';
import OperatorIcon from '../../operator-icon';
import useGraphStore from '../../store';
import CategorizeHandle from './categorize-handle';
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

  const isCategorize = data.label === Operator.Categorize;
  const categoryData = get(data, 'form.category_description') ?? {};

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
      {isCategorize &&
        Object.keys(categoryData).map((x, idx) => (
          <CategorizeHandle
            top={CategorizeAnchorPointPositions[idx].top}
            right={CategorizeAnchorPointPositions[idx].right}
            key={idx}
            text={x}
            idx={idx}
          ></CategorizeHandle>
        ))}
      <Flex vertical align="center" justify="center">
        <Space size={6}>
          <OperatorIcon
            name={data.label as Operator}
            fontSize={16}
          ></OperatorIcon>
          <OperateDropdown
            iconFontSize={14}
            deleteItem={deleteNode}
            items={items}
          ></OperateDropdown>
        </Space>
      </Flex>

      <section className={styles.bottomBox}>
        <div className={styles.nodeName}>{id}</div>
      </section>
    </section>
  );
}
