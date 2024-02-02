import { ReactComponent as MoreIcon } from '@/assets/svg/more.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { formatDate } from '@/utils/date';
import {
  CalendarOutlined,
  DeleteOutlined,
  FileTextOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Avatar, Card, Dropdown, MenuProps, Space } from 'antd';
import { MouseEvent } from 'react';
import { useDispatch, useNavigate } from 'umi';

import styles from './index.less';

interface IProps {
  item: any;
}

const KnowledgeCard = ({ item }: IProps) => {
  const navigate = useNavigate();
  const dispatch = useDispatch();

  const handleDelete = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
  };

  const items: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <Space>
          删除
          <DeleteOutlined onClick={handleDelete} />
        </Space>
      ),
    },
  ];

  const confirm = (id: string) => {
    dispatch({
      type: 'knowledgeModel/rmKb',
      payload: {
        kb_id: id,
      },
    });
  };

  const handleCardClick = () => {
    navigate(`/knowledge/${KnowledgeRouteKey.Dataset}?id=${item.id}`);
  };

  const onConfirmDelete = (e?: MouseEvent<HTMLElement>) => {
    e?.stopPropagation();
    e?.nativeEvent.stopImmediatePropagation();
    confirm(item.id);
  };

  return (
    <Card className={styles.card} onClick={handleCardClick}>
      <div className={styles.container}>
        <div className={styles.content}>
          <Avatar size={34} icon={<UserOutlined />} />

          <span className={styles.delete}>
            {/* <Popconfirm
              title="Delete the task"
              description="Are you sure to delete this task?"
              onConfirm={onConfirmDelete}
              okText="Yes"
              cancelText="No"
            >
              <DeleteOutlined onClick={handleDelete} />
            </Popconfirm> */}
            <Dropdown menu={{ items }}>
              <MoreIcon />
            </Dropdown>
          </span>
        </div>
        <div className={styles.titleWrapper}>
          <span className={styles.title}>{item.name}</span>
          <p>A comprehensive knowledge base for crafting effective resumes.</p>
        </div>
        <div className={styles.footer}>
          <div className={styles.footerTop}>
            <div className={styles.bottomLeft}>
              <FileTextOutlined className={styles.leftIcon} />
              <span className={styles.rightText}>
                <Space>{item.doc_num}文档</Space>
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
