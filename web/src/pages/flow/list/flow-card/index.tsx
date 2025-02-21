import { formatDate } from '@/utils/date';
import { CalendarOutlined } from '@ant-design/icons';
import { Card, Typography } from 'antd';
import { useCallback, useState } from 'react';
import { useNavigate } from 'umi';

import OperateDropdown from '@/components/operate-dropdown';
import RenameModal from '@/components/rename-modal';
import { useDeleteFlow, useUpdateFlowName } from '@/hooks/flow-hooks';
import { IFlow } from '@/interfaces/database/flow';
import GraphAvatar from '../graph-avatar';
import styles from './index.less';

interface IProps {
  item: IFlow;
}

const FlowCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const { deleteFlow } = useDeleteFlow();
  const { updateFlowName } = useUpdateFlowName();
  const [isRenameModalVisible, setIsRenameModalVisible] = useState(false);
  const [renameLoading, setRenameLoading] = useState(false);

  const removeFlow = useCallback(() => {
    return deleteFlow([item.id]);
  }, [deleteFlow, item]);

  const handleUpdateName = useCallback(() => {
    setIsRenameModalVisible(true);
  }, []);

  const handleRenameOk = async (newName: string) => {
    setRenameLoading(true);
    try {
      const success = await updateFlowName(item.id, newName);
      if (success) {
        setIsRenameModalVisible(false);
      }
    } finally {
      setRenameLoading(false);
    }
  };

  const handleCardClick = () => {
    navigate(`/flow/${item.id}`);
  };

  return (
    <>
      <Card className={styles.card} onClick={handleCardClick}>
        <div className={styles.container}>
          <div className={styles.content}>
            <GraphAvatar avatar={item.avatar}></GraphAvatar>
            <OperateDropdown
              deleteItem={removeFlow}
              onUpdateName={handleUpdateName}
            ></OperateDropdown>
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

      <RenameModal
        visible={isRenameModalVisible}
        hideModal={() => setIsRenameModalVisible(false)}
        loading={renameLoading}
        initialName={item.title}
        onOk={handleRenameOk}
      />
    </>
  );
};

export default FlowCard;
