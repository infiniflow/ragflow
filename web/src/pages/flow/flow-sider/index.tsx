import { Avatar, Card, Flex, Layout, Space } from 'antd';
import classNames from 'classnames';
import { useState } from 'react';
import { componentList } from '../mock';

import { useHandleDrag } from '../hooks';
import styles from './index.less';

const { Sider } = Layout;

const FlowSider = () => {
  const [collapsed, setCollapsed] = useState(true);
  const { handleDrag } = useHandleDrag();

  return (
    <Sider
      collapsible
      collapsed={collapsed}
      collapsedWidth={0}
      theme={'light'}
      onCollapse={(value) => setCollapsed(value)}
    >
      <Flex vertical gap={10} className={styles.siderContent}>
        {componentList.map((x) => (
          <Card
            key={x.name}
            hoverable
            draggable
            className={classNames(styles.operatorCard)}
            onDragStart={handleDrag(x.name)}
          >
            <Flex justify="space-between" align="center">
              <Space size={15}>
                <Avatar icon={x.icon} shape={'square'} />
                <section>
                  <b>{x.name}</b>
                  <div>{x.description}</div>
                </section>
              </Space>
            </Flex>
          </Card>
        ))}
      </Flex>
    </Sider>
  );
};

export default FlowSider;
