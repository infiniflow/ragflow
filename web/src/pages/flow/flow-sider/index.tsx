import { useTranslate } from '@/hooks/common-hooks';
import { Card, Divider, Flex, Layout, Tooltip } from 'antd';
import classNames from 'classnames';
import lowerFirst from 'lodash/lowerFirst';
import React from 'react';
import { Operator, componentMenuList, operatorMap } from '../constant';
import { useHandleDrag } from '../hooks';
import OperatorIcon from '../operator-icon';
import styles from './index.less';

const { Sider } = Layout;

interface IProps {
  setCollapsed: (width: boolean) => void;
  collapsed: boolean;
}

const dividerProps = {
  marginTop: 10,
  marginBottom: 10,
  padding: 0,
  borderBlockColor: '#b4afaf',
  borderStyle: 'dotted',
};

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
            <React.Fragment key={x.name}>
              {x.name === Operator.Note && (
                <Divider style={dividerProps}></Divider>
              )}
              {x.name === Operator.DuckDuckGo && (
                <Divider style={dividerProps}></Divider>
              )}
              <Card
                key={x.name}
                hoverable
                draggable
                className={classNames(styles.operatorCard)}
                onDragStart={handleDragStart(x.name)}
              >
                <Flex align="center" gap={15}>
                  <OperatorIcon
                    name={x.name}
                    color={operatorMap[x.name].color}
                  ></OperatorIcon>
                  <section>
                    <Tooltip title={t(`${lowerFirst(x.name)}Description`)}>
                      <b>{t(lowerFirst(x.name))}</b>
                    </Tooltip>
                  </section>
                </Flex>
              </Card>
            </React.Fragment>
          );
        })}
      </Flex>
    </Sider>
  );
};

export default FlowSide;
