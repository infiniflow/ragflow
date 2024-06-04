import { ReactComponent as MoreIcon } from '@/assets/svg/more.svg';
import { useShowDeleteConfirm } from '@/hooks/commonHooks';
import { formatDate } from '@/utils/date';
import {
  CalendarOutlined,
  DeleteOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Card, Dropdown, MenuProps, Space } from 'antd';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import { useDeleteFlow } from '@/hooks/flow-hooks';
import { IFlow } from '../../interface';
import styles from './index.less';

interface IProps {
  item: IFlow;
}

const FlowCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const showDeleteConfirm = useShowDeleteConfirm();
  const { t } = useTranslation();
  const { deleteFlow } = useDeleteFlow();

  const removeKnowledge = () => {
    return deleteFlow([item.id]);
  };

  const handleDelete = () => {
    showDeleteConfirm({ onOk: removeKnowledge });
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

  const handleDropdownMenuClick: MenuProps['onClick'] = ({ domEvent, key }) => {
    domEvent.preventDefault();
    domEvent.stopPropagation();
    if (key === '1') {
      handleDelete();
    }
  };

  const handleCardClick = () => {
    navigate(`/flow/${item.id}`);
  };

  return (
    <Card className={styles.card} onClick={handleCardClick}>
      <div className={styles.container}>
        <div className={styles.content}>
          <Avatar size={34} icon={<UserOutlined />} src={item.avatar} />
          <Dropdown
            menu={{
              items,
              onClick: handleDropdownMenuClick,
            }}
          >
            <span className={styles.delete}>
              <MoreIcon />
            </span>
          </Dropdown>
        </div>
        <div className={styles.titleWrapper}>
          <span className={styles.title}>{item.title}</span>
          <p>{item.description}</p>
        </div>
        <div className={styles.footer}>
          <div className={styles.bottom}>
            <div className={styles.bottomLeft}>
              <CalendarOutlined className={styles.leftIcon} />
              <span className={styles.rightText}>
                {formatDate(item.update_time)}
              </span>
            </div>
          </div>
        </div>
      </div>
    </Card>
  );
};

export default FlowCard;
