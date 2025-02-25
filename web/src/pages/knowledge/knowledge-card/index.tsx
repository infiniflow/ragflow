import { KnowledgeRouteKey } from '@/constants/knowledge';
import { IKnowledge } from '@/interfaces/database/knowledge';
import { formatDate } from '@/utils/date';
import {
  CalendarOutlined,
  FileTextOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Badge, Card, Space } from 'antd';
import classNames from 'classnames';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import OperateDropdown from '@/components/operate-dropdown';
import RenameModal from '@/components/rename-modal';
import { useTheme } from '@/components/theme-provider';
import {
  useDeleteKnowledge,
  useUpdateKnowledgeName,
} from '@/hooks/knowledge-hooks';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import styles from './index.less';

interface IProps {
  item: IKnowledge;
}

const KnowledgeCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const { data: userInfo } = useFetchUserInfo();
  const { theme } = useTheme();
  const { deleteKnowledge } = useDeleteKnowledge();
  const { updateKnowledgeName } = useUpdateKnowledgeName();
  const [isRenameModalVisible, setIsRenameModalVisible] = useState(false);
  const [renameLoading, setRenameLoading] = useState(false);

  const removeKnowledge = async () => {
    return deleteKnowledge(item.id);
  };

  const handleCardClick = () => {
    navigate(`/knowledge/${KnowledgeRouteKey.Dataset}?id=${item.id}`, {
      state: { from: 'list' },
    });
  };

  const renameKnowledge = () => {
    setIsRenameModalVisible(true);
  };

  const renameKnowledgeOk = async (newName: string) => {
    setRenameLoading(true);
    try {
      console.log('Renaming knowledge:', { id: item.id, newName });
      const success = await updateKnowledgeName(item.id, newName);
      console.log('Rename result:', success);
      if (success) {
        setIsRenameModalVisible(false);
      }
    } catch (error) {
      console.error('Rename error:', error);
    } finally {
      setRenameLoading(false);
    }
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
            <Avatar size={34} icon={<UserOutlined />} src={item.avatar} />
            <OperateDropdown
              deleteItem={removeKnowledge}
              updateItem={renameKnowledge}
            ></OperateDropdown>
          </div>
          <div className={styles.titleWrapper}>
            <span
              className={theme === 'dark' ? styles.titledark : styles.title}
            >
              {item.name}
            </span>
            <p
              className={
                theme === 'dark' ? styles.descriptiondark : styles.description
              }
            >
              {item.description}
            </p>
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

      <RenameModal
        visible={isRenameModalVisible}
        hideModal={() => setIsRenameModalVisible(false)}
        loading={renameLoading}
        initialName={item.name}
        onOk={renameKnowledgeOk}
      />
    </Badge.Ribbon>
  );
};

export default KnowledgeCard;
