import { formatDate } from '@/utils/date';
import { CalendarOutlined } from '@ant-design/icons';
import { Badge, Card, Typography } from 'antd';
import { useNavigate } from 'umi';

import OperateDropdown from '@/components/operate-dropdown';
import { useDeleteFlow } from '@/hooks/flow-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { IFlow } from '@/interfaces/database/flow';
import classNames from 'classnames';
import { useCallback } from 'react';
import GraphAvatar from '../graph-avatar';
import styles from './index.less';

interface IProps {
  item: IFlow;
  onDelete?: (string: string) => void;
}

const FlowCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const { deleteFlow } = useDeleteFlow();
  const { data: userInfo } = useFetchUserInfo();

  const removeFlow = useCallback(() => {
    return deleteFlow([item.id]);
  }, [deleteFlow, item]);

  const handleCardClick = () => {
    navigate(`/flow/${item.id}`);
  };

  return (
    <Badge.Ribbon
      text={item?.nickname}
      color={userInfo?.nickname === item?.nickname ? '#1677ff' : 'pink'}
      className={classNames(styles.ribbon, {
        [styles.hideRibbon]: item.permission !== 'team',
      })}
    >
      <Card className={styles.card} onClick={handleCardClick}>
        <div className={styles.container}>
          <div className={styles.content}>
            <GraphAvatar avatar={item.avatar}></GraphAvatar>
            <OperateDropdown deleteItem={removeFlow}></OperateDropdown>
          </div>
          <div className={styles.titleWrapper}>
            <Typography.Title
              className={styles.title}
              ellipsis={{ tooltip: item.title }}
            >
              {item.title}
            </Typography.Title>
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
    </Badge.Ribbon>
  );
};

export default FlowCard;
