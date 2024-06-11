import { Card, Flex, Layout, Space } from 'antd';
import classNames from 'classnames';

import { componentList } from '../constant';
import { useHandleDrag } from '../hooks';
import OperatorIcon from '../operator-icon';
import styles from './index.less';

const { Sider } = Layout;

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
        {componentList.map((x) => {
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
                  {/* <Avatar
                    icon={<OperatorIcon name={x.name}></OperatorIcon>}
                    shape={'square'}
                  /> */}
                  <section>
                    <b>{x.name}</b>
                    <div>{x.description}</div>
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
