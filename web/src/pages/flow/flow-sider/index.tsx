import { Card, Flex, Layout, Space, Typography } from 'antd';
import classNames from 'classnames';

import { componentMenuList } from '../constant';
import { useHandleDrag } from '../hooks';
import OperatorIcon from '../operator-icon';
import styles from './index.less';

const { Sider } = Layout;

const { Text } = Typography;

interface IProps {
  setCollapsed: (width: boolean) => void;
  collapsed: boolean;
}

const FlowSide = ({ setCollapsed, collapsed }: IProps) => {
  const { handleDragStart } = useHandleDrag();

  return (
    <Sider
      collapsible
      collapsed={collapsed}
      collapsedWidth={0}
      theme={'light'}
      onCollapse={(value) => setCollapsed(value)}
    >
      <Flex vertical gap={10} className={styles.siderContent}>
        {componentMenuList.map((x) => {
          return (
            <Card
              key={x.name}
              hoverable
              draggable
              className={classNames(styles.operatorCard)}
              onDragStart={handleDragStart(x.name)}
            >
              <Flex justify="space-between" align="center">
                <Space size={15}>
                  <OperatorIcon name={x.name}></OperatorIcon>
                  <section>
                    <b>{x.name}</b>
                    <Text
                      ellipsis={{ tooltip: x.description }}
                      style={{ width: 130 }}
                    >
                      {x.description}
                    </Text>
                  </section>
                </Space>
              </Flex>
            </Card>
          );
        })}
      </Flex>
    </Sider>
  );
};

export default FlowSide;
