import { useTranslate } from '@/hooks/commonHooks';
import { Card, Flex, Layout, Space, Tooltip } from 'antd';
import classNames from 'classnames';
import lowerFirst from 'lodash/lowerFirst';
import { componentMenuList } from '../constant';
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
  const { t } = useTranslate('flow');

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
                    <Tooltip title={t(`${lowerFirst(x.name)}Description`)}>
                      <b>{x.name}</b>
                    </Tooltip>
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
