import { formatDate } from '@/utils/date';
import { CalendarOutlined, UserOutlined } from '@ant-design/icons';
import { Avatar, Card } from 'antd';
import { useNavigate } from 'umi';

import OperateDropdown from '@/components/operate-dropdown';
import { useDeleteFlow } from '@/hooks/flow-hooks';
import { IFlow } from '@/interfaces/database/flow';
import { useCallback } from 'react';
import styles from './index.less';

interface IProps {
  item: IFlow;
}

const FlowCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const { deleteFlow } = useDeleteFlow();

  const removeFlow = useCallback(() => {
    return deleteFlow([item.id]);
  }, [deleteFlow, item]);

  const handleCardClick = () => {
    navigate(`/flow/${item.id}`);
  };

  return (
    <Card className={styles.card} onClick={handleCardClick}>
      <div className={styles.container}>
        <div className={styles.content}>
          <Avatar size={34} icon={<UserOutlined />} src={item.avatar} />
          <OperateDropdown deleteItem={removeFlow}></OperateDropdown>
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
