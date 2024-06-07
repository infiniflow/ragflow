import { ReactComponent as MoreIcon } from '@/assets/svg/more.svg';
import { useShowDeleteConfirm } from '@/hooks/commonHooks';
import { DeleteOutlined } from '@ant-design/icons';
import { Dropdown, MenuProps, Space } from 'antd';
import { useTranslation } from 'react-i18next';

import React from 'react';
import styles from './index.less';

interface IProps {
  deleteItem: () => Promise<any>;
}

const OperateDropdown = ({
  deleteItem,
  children,
}: React.PropsWithChildren<IProps>) => {
  const { t } = useTranslation();
  const showDeleteConfirm = useShowDeleteConfirm();

  const handleDelete = () => {
    showDeleteConfirm({ onOk: deleteItem });
  };

  const handleDropdownMenuClick: MenuProps['onClick'] = ({ domEvent, key }) => {
    domEvent.preventDefault();
    domEvent.stopPropagation();
    if (key === '1') {
      handleDelete();
    }
  };

  const items: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <Space>
          {t('common.delete')}
          <DeleteOutlined />
        </Space>
      ),
    },
  ];

  return (
    <Dropdown
      menu={{
        items,
        onClick: handleDropdownMenuClick,
      }}
    >
      {children || (
        <span className={styles.delete}>
          <MoreIcon />
        </span>
      )}
    </Dropdown>
  );
};

export default OperateDropdown;
