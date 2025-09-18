import OperateDropdown from '@/components/operate-dropdown';
import { CopyOutlined } from '@ant-design/icons';
import { Flex, MenuProps } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Operator } from '../../constant';
import { useDuplicateNode } from '../../hooks';
import useGraphStore from '../../store';

interface IProps {
  id: string;
  iconFontColor?: string;
  label: string;
}

const NodeDropdown = ({ id, iconFontColor, label }: IProps) => {
  const { t } = useTranslation();
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const deleteIterationNodeById = useGraphStore(
    (store) => store.deleteIterationNodeById,
  );

  const deleteNode = useCallback(() => {
    if (label === Operator.Iteration) {
      deleteIterationNodeById(id);
    } else {
      deleteNodeById(id);
    }
  }, [label, deleteIterationNodeById, id, deleteNodeById]);

  const duplicateNode = useDuplicateNode();

  const items: MenuProps['items'] = [
    {
      key: '2',
      onClick: () => duplicateNode(id, label),
      label: (
        <Flex justify={'space-between'}>
          {t('common.copy')}
          <CopyOutlined />
        </Flex>
      ),
    },
  ];

  return (
    <OperateDropdown
      iconFontSize={22}
      height={14}
      deleteItem={deleteNode}
      items={items}
      needsDeletionValidation={false}
      iconFontColor={iconFontColor}
    ></OperateDropdown>
  );
};

export default NodeDropdown;
