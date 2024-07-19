import { KnowledgeRouteKey } from '@/constants/knowledge';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import {
  CalendarOutlined,
  FileTextOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Card, Space } from 'antd';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import OperateDropdown from '@/components/operate-dropdown';
import { useDeleteKnowledge } from '@/hooks/knowledge-hooks';
import styles from './index.less';

interface IProps {
  item: IKnowledge;
}

const KnowledgeCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const { t } = useTranslation();

  const { deleteKnowledge } = useDeleteKnowledge();

  const removeKnowledge = async () => {
    return deleteKnowledge(item.id);
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
          <OperateDropdown deleteItem={removeKnowledge}></OperateDropdown>
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
                {formatDate(item.update_time)}
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
