import { useTranslation } from 'react-i18next';

import { useEffect, useState } from 'react';
import styles from './index.less';

import { RadarChartOutlined } from '@ant-design/icons';
import { ProCard } from '@ant-design/pro-components';
import { Button, Card, Col, Row, Tag } from 'antd';
import { useDispatch, useSelector } from 'umi';

interface DataType {
  key: React.Key;
  name: string;
  age: number;
  address: string;
  description: string;
}

const SettingList = () => {
  const dispatch = useDispatch();
  const settingModel = useSelector((state: any) => state.settingModel);
  const { llmInfo = {}, factoriesList, myLlm = [] } = settingModel;
  const { OpenAI = [], tongyi = [] } = llmInfo;
  const [collapsed, setCollapsed] = useState(true);
  const { t } = useTranslation();

  useEffect(() => {
    dispatch({
      type: 'settingModel/factories_list',
      payload: {},
    });
    dispatch({
      type: 'settingModel/llm_list',
      payload: {},
    });
    dispatch({
      type: 'settingModel/my_llm',
      payload: {},
    });
  }, [dispatch]);

  return (
    <div
      className={styles.list}
      style={{
        display: 'flex',
        flexDirection: 'column',
        padding: 24,
        gap: 12,
      }}
    >
      {myLlm.map((item: any) => {
        return (
          <ProCard
            key={item.llm_factory}
            // title={<div>可折叠-图标自定义</div>}
            collapsibleIconRender={({
              collapsed: buildInCollapsed,
            }: {
              collapsed: boolean;
            }) => {
              return (
                <div>
                  <h3>
                    <RadarChartOutlined />
                    {item.llm_factory}
                  </h3>
                  <div>
                    {item.tags.split(',').map((d: string) => {
                      return <Tag key={d}>{d}</Tag>;
                    })}
                  </div>
                  {buildInCollapsed ? (
                    <span>显示{OpenAI.length}个模型</span>
                  ) : (
                    <span>收起{OpenAI.length}个模型 </span>
                  )}
                </div>
              );
            }}
            extra={
              <Button
                size="small"
                type="link"
                onClick={(e) => {
                  e.stopPropagation();
                  dispatch({
                    type: 'settingModel/updateState',
                    payload: {
                      llm_factory: item.llm_factory,
                      isShowSAKModal: true,
                    },
                  });
                }}
              >
                设置
              </Button>
            }
            style={{ marginBlockStart: 16 }}
            headerBordered
            collapsible
            defaultCollapsed
          ></ProCard>
        );
      })}

      <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 32 }}>
        {factoriesList.map((item: any) => {
          return (
            <Col key={item.name} xs={24} sm={12} md={8} lg={6}>
              <Card
                title={item.name}
                bordered={false}
                extra={
                  <Button
                    size="small"
                    type="link"
                    onClick={(e) => {
                      e.stopPropagation();
                      dispatch({
                        type: 'settingModel/updateState',
                        payload: {
                          llm_factory: item.name,
                          isShowSAKModal: true,
                        },
                      });
                    }}
                  >
                    设置
                  </Button>
                }
              >
                <div>
                  {item.tags.split(',').map((d: string) => {
                    return <Tag key={d}>{d}</Tag>;
                  })}
                </div>
              </Card>
            </Col>
          );
        })}
      </Row>
    </div>
  );
};
export default SettingList;
