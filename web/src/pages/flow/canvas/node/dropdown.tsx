import OperateDropdown from '@/components/operate-dropdown';
import { CopyOutlined } from '@ant-design/icons';
import { Flex, MenuProps } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import useGraphStore from '../../store';

interface IProps {
  id: string;
}

const NodeDropdown = ({ id }: IProps) => {
  const { t } = useTranslation();
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const duplicateNodeById = useGraphStore((store) => store.duplicateNode);

  const deleteNode = useCallback(() => {
    deleteNodeById(id);
  }, [id, deleteNodeById]);

  const duplicateNode = useCallback(() => {
    duplicateNodeById(id);
  }, [id, duplicateNodeById]);

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
    <OperateDropdown
      iconFontSize={14}
      height={14}
      deleteItem={deleteNode}
      items={items}
      needsDeletionValidation={false}
    ></OperateDropdown>
  );
};

export default NodeDropdown;
