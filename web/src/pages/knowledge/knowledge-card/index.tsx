import { ReactComponent as MoreIcon } from '@/assets/svg/more.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useShowDeleteConfirm } from '@/hooks/commonHooks';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import {
  CalendarOutlined,
  DeleteOutlined,
  FileTextOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Card, Dropdown, MenuProps, Space } from 'antd';
import { useTranslation } from 'react-i18next';
import { useDispatch, useNavigate } from 'umi';

import styles from './index.less';

interface IProps {
  item: IKnowledge;
}

const KnowledgeCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const dispatch = useDispatch();
  const showDeleteConfirm = useShowDeleteConfirm();
  const { t } = useTranslation();

  const removeKnowledge = () => {
    return dispatch({
      type: 'knowledgeModel/rmKb',
      payload: {
        kb_id: item.id,
      },
    });
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
    navigate(`/knowledge/${KnowledgeRouteKey.Dataset}?id=${item.id}`, {
      state: { from: 'list' },
    });
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
          <span className={styles.title}>{item.name}</span>
          <p>{item.description}</p>
        </div>
        <div className={styles.footer}>
          <div className={styles.footerTop}>
            <div className={styles.bottomLeft}>
              <FileTextOutlined className={styles.leftIcon} />
              <span className={styles.rightText}>
                <Space>
                  {item.doc_num}
                  {t('knowledgeList.doc')}
                </Space>
              </span>
            </div>
          </div>
          <div className={styles.bottom}>
            <div className={styles.bottomLeft}>
              <CalendarOutlined className={styles.leftIcon} />
              <span className={styles.rightText}>
                {formatDate(item.update_date)}
              </span>
            </div>
            {/* <Avatar.Group size={25}>
              <Avatar src="https://api.dicebear.com/7.x/miniavs/svg?seed=1" />
              <a href="https://ant.design">
                <Avatar style={{ backgroundColor: '#f56a00' }}>K</Avatar>
              </a>
              <Tooltip title="Ant User" placement="top">
                <Avatar
                  style={{ backgroundColor: '#87d068' }}
                  icon={<UserOutlined />}
                />
              </Tooltip>
              <Avatar
                style={{ backgroundColor: '#1677ff' }}
                icon={<AntDesignOutlined />}
              />
            </Avatar.Group> */}
          </div>
        </div>
      </div>
    </Card>
  );
};

export default KnowledgeCard;
