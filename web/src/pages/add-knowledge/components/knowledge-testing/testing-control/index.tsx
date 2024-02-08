import { Button, Card, Divider, Flex, Input, Space, Tag } from 'antd';

import { DeleteOutlined, HistoryOutlined } from '@ant-design/icons';
import styles from './index.less';

const list = [1, 2, 3];

const TestingControl = () => {
  return (
    <section className={styles.testingControlWrapper}>
      <p>
        <b>Retrieval testing</b>
      </p>
      <p>xxxx</p>
      <Divider></Divider>
      <section>
        <Card
          size="small"
          title="Test text"
          extra={
            <Button type="primary" ghost>
              Semantic Search
            </Button>
          }
        >
          <Input.TextArea autoSize={{ minRows: 8 }}></Input.TextArea>
          <Flex justify={'space-between'}>
            <Tag>10/200</Tag>
            <Button type="primary" size="small">
              Testing
            </Button>
          </Flex>
        </Card>
      </section>
      <section>
        <p className={styles.historyTitle}>
          <Space size={'middle'}>
            <HistoryOutlined className={styles.historyIcon} />
            <b>Test history</b>
          </Space>
        </p>
        <Space
          direction="vertical"
          size={'middle'}
          className={styles.historyCardWrapper}
        >
          {list.map((x) => (
            <Card className={styles.historyCard} key={x}>
              <Flex justify={'space-between'} gap={'small'}>
                <span>{x}</span>
                <div className={styles.historyText}>
                  content dcjsjl snldsh svnodvn svnodrfn svjdoghdtbnhdo
                  sdvhodhbuid sldghdrlh
                </div>
                <Flex gap={'small'}>
                  <span>time</span>
                  <DeleteOutlined></DeleteOutlined>
                </Flex>
              </Flex>
            </Card>
          ))}
        </Space>
      </section>
    </section>
  );
};

export default TestingControl;
