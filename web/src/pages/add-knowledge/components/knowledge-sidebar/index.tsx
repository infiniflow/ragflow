import { getWidth } from '@/utils';
import {
  AntDesignOutlined,
  BarsOutlined,
  SearchOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Avatar, Divider, Menu, MenuProps, Space } from 'antd';
import classNames from 'classnames';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams, useSelector } from 'umi';
import styles from './index.less';

const KnowledgeSidebar = () => {
  const kAModel = useSelector((state: any) => state.kAModel);
  const { id } = kAModel;
  let navigate = useNavigate();
  const params = useParams();
  const activeKey = params.module || 'file';

  const [windowWidth, setWindowWidth] = useState(getWidth());
  const [collapsed, setCollapsed] = useState(false);

  const handleSelect: MenuProps['onSelect'] = (e) => {
    navigate(`/knowledge/${e.key}?id=${id}`);
  };

  type MenuItem = Required<MenuProps>['items'][number];

  function getItem(
    label: React.ReactNode,
    key: React.Key,
    icon?: React.ReactNode,
    disabled?: boolean,
    children?: MenuItem[],
    type?: 'group',
  ): MenuItem {
    return {
      key,
      icon,
      children,
      label,
      type,
      disabled,
    } as MenuItem;
  }
  const items: MenuItem[] = useMemo(() => {
    // const disabled = !id;
    return [
      getItem('配置', 'setting', <ToolOutlined />),
      getItem('知识库', 'file', <BarsOutlined />),
      getItem('搜索测试', 'search', <SearchOutlined />),
    ];
  }, [id]);

  useEffect(() => {
    if (windowWidth.width > 957) {
      setCollapsed(false);
    } else {
      setCollapsed(true);
    }
  }, [windowWidth.width]);

  // 标记一下
  useEffect(() => {
    const widthSize = () => {
      const width = getWidth();
      console.log(width);

      setWindowWidth(width);
    };
    window.addEventListener('resize', widthSize);
    return () => {
      window.removeEventListener('resize', widthSize);
    };
  }, []);

  return (
    <div className={styles.sidebarWrapper}>
      <div className={styles.sidebarTop}>
        <Space size={8} direction="vertical">
          <Avatar size={64} icon={<AntDesignOutlined />} />
          <div className={styles.knowledgeTitle}>Cloud Computing</div>
        </Space>
        <p className={styles.knowledgeDescription}>
          A scalable, secure cloud-based database optimized for high-performance
          computing and data storage.
        </p>
      </div>
      <Divider dashed />
      <div className={styles.menu}>
        <Menu
          selectedKeys={[activeKey]}
          mode="inline"
          className={classNames({
            [styles.defaultWidth]: windowWidth.width > 957,
            [styles.minWidth]: windowWidth.width <= 957,
          })}
          inlineCollapsed={collapsed}
          items={items}
          onSelect={handleSelect}
        />
      </div>
    </div>
  );
};

export default KnowledgeSidebar;
