import OperateDropdown from '@/components/operate-dropdown';
import { CopyOutlined } from '@ant-design/icons';
import { Flex, MenuProps } from 'antd';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useGetNodeName } from '../../hooks';
import useGraphStore from '../../store';

interface IProps {
  id: string;
  iconFontColor?: string;
  label: string;
}

const NodeDropdown = ({ id, iconFontColor, label }: IProps) => {
  const { t } = useTranslation();
  const deleteNodeById = useGraphStore((store) => store.deleteNodeById);
  const duplicateNodeById = useGraphStore((store) => store.duplicateNode);
  const getNodeName = useGetNodeName();

  const deleteNode = useCallback(() => {
    deleteNodeById(id);
  }, [id, deleteNodeById]);

  const duplicateNode = useCallback(() => {
    duplicateNodeById(id, getNodeName(label));
  }, [duplicateNodeById, id, getNodeName, label]);

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
