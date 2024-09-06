import HightLightMarkdown from '@/components/highlight-markdown';
import { ImageWithPopover } from '@/components/image';
import MessageItem from '@/components/message-item';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { IReference } from '@/interfaces/database/chat';
import { Card, Flex, Input, Layout, List, Space } from 'antd';
import { useState } from 'react';
import { useSendQuestion } from './hooks';
import SearchSidebar from './sidebar';

import styles from './index.less';

const { Content } = Layout;
const { Search } = Input;

const SearchPage = () => {
  const [checkedList, setCheckedList] = useState<string[]>([]);
  const list = useSelectTestingResult();
  const { sendQuestion, message, sendingLoading } =
    useSendQuestion(checkedList);

  return (
    <Layout className={styles.searchPage}>
      <SearchSidebar
        checkedList={checkedList}
        setCheckedList={setCheckedList}
      ></SearchSidebar>
      <Layout>
        <Content>
          <Flex className={styles.content}>
            <section className={styles.main}>
              <Search
                placeholder="input search text"
                onSearch={sendQuestion}
                size="large"
              />
              <MessageItem
                item={message}
                nickname="You"
                reference={message.reference ?? ({} as IReference)}
                loading={sendingLoading}
                index={0}
              ></MessageItem>
              <List
                dataSource={list.chunks}
                renderItem={(item) => (
                  <List.Item>
                    <Card>
                      <Space>
                        <ImageWithPopover id={item.img_id}></ImageWithPopover>
                        <HightLightMarkdown>
                          {item.highlight}
                        </HightLightMarkdown>
                      </Space>
                    </Card>
                  </List.Item>
                )}
              />
            </section>
            <section className={styles.graph}></section>
          </Flex>
        </Content>
      </Layout>
    </Layout>
  );
};

export default SearchPage;
