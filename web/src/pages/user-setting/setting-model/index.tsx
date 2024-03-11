import {
  useFetchLlmFactoryListOnMount,
  useFetchMyLlmListOnMount,
} from '@/hooks/llmHooks';
import { SettingOutlined } from '@ant-design/icons';
import {
  Avatar,
  Button,
  Card,
  Col,
  Divider,
  Flex,
  List,
  Row,
  Space,
  Tag,
} from 'antd';
import SettingTitle from '../components/setting-title';

import styles from './index.less';

const UserSettingModel = () => {
  const factoryList = useFetchLlmFactoryListOnMount();
  const llmList = useFetchMyLlmListOnMount();

  return (
    <section className={styles.modelWrapper}>
      <SettingTitle
        title="Model Setting"
        description="Manage your account settings and preferences here."
      ></SettingTitle>
      <Divider></Divider>
      <List
        grid={{ gutter: 16, column: 1 }}
        dataSource={llmList}
        renderItem={(item) => (
          <List.Item>
            <Card>
              <Row align={'middle'}>
                <Col span={12}>
                  <Flex gap={'middle'} align="center">
                    <Avatar shape="square" size="large" src={item.logo} />
                    <Flex vertical gap={'small'}>
                      <b>{item.name}</b>
                      <div>
                        {item.tags.split(',').map((x) => (
                          <Tag key={x}>{x}</Tag>
                        ))}
                      </div>
                    </Flex>
                  </Flex>
                </Col>
                <Col span={12} className={styles.factoryOperationWrapper}>
                  <Space size={'middle'}>
                    <Button>
                      API-Key
                      <SettingOutlined />
                    </Button>
                    <Button>
                      Show more models
                      <SettingOutlined />
                    </Button>
                  </Space>
                </Col>
              </Row>
              <List
                size="small"
                dataSource={item.llm}
                renderItem={(item) => <List.Item>{item.name}</List.Item>}
              />
            </Card>
          </List.Item>
        )}
      />
      <p>Models to be added</p>
      <List
        grid={{
          gutter: 16,
          xs: 1,
          sm: 2,
          md: 3,
          lg: 4,
          xl: 4,
          xxl: 8,
        }}
        dataSource={factoryList}
        renderItem={(item) => (
          <List.Item>
            <Card>
              <Flex vertical gap={'large'}>
                <Avatar shape="square" size="large" src={item.logo} />
                <Flex vertical gap={'middle'}>
                  <b>{item.name}</b>
                  <Space wrap>
                    {item.tags.split(',').map((x) => (
                      <Tag key={x}>{x}</Tag>
                    ))}
                  </Space>
                </Flex>
              </Flex>
            </Card>
          </List.Item>
        )}
      />
    </section>
  );
};

export default UserSettingModel;
