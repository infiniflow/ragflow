import HightLightMarkdown from '@/components/highlight-markdown';
import { ImageWithPopover } from '@/components/image';
import IndentedTree from '@/components/indented-tree/indented-tree';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { IReference } from '@/interfaces/database/chat';
import {
  Card,
  Divider,
  Flex,
  Input,
  Layout,
  List,
  Skeleton,
  Space,
  Tag,
} from 'antd';
import { useState } from 'react';
import MarkdownContent from '../chat/markdown-content';
import { useSendQuestion } from './hooks';
import SearchSidebar from './sidebar';

import styles from './index.less';

const { Content } = Layout;
const { Search } = Input;

const SearchPage = () => {
  const [checkedList, setCheckedList] = useState<string[]>([]);
  const list = useSelectTestingResult();
  const {
    sendQuestion,
    handleClickRelatedQuestion,
    handleSearchStrChange,
    answer,
    sendingLoading,
    relatedQuestions,
    mindMap,
    mindMapLoading,
    searchStr,
    loading,
    isFirstRender,
  } = useSendQuestion(checkedList);

  const InputSearch = (
    <Search
      value={searchStr}
      onChange={handleSearchStrChange}
      placeholder="input search text"
      allowClear
      enterButton
      onSearch={sendQuestion}
      size="large"
      loading={sendingLoading}
      disabled={checkedList.length === 0}
      className={isFirstRender ? styles.globalInput : styles.partialInput}
    />
  );

  return (
    <Layout className={styles.searchPage}>
      <SearchSidebar
        checkedList={checkedList}
        setCheckedList={setCheckedList}
      ></SearchSidebar>
      <Layout>
        <Content>
          {isFirstRender ? (
            <Flex
              justify="center"
              align="center"
              className={styles.firstRenderContent}
            >
              {InputSearch}
            </Flex>
          ) : (
            <Flex className={styles.content}>
              <section className={styles.main}>
                {InputSearch}
                {answer.answer && (
                  <div className={styles.answerWrapper}>
                    <MarkdownContent
                      loading={sendingLoading}
                      content={answer.answer}
                      reference={answer.reference ?? ({} as IReference)}
                      clickDocumentButton={() => {}}
                    ></MarkdownContent>
                  </div>
                )}
                <Divider></Divider>
                {list.chunks.length > 0 && (
                  <List
                    dataSource={list.chunks}
                    loading={loading}
                    renderItem={(item) => (
                      <List.Item>
                        <Card className={styles.card}>
                          <Space>
                            <ImageWithPopover
                              id={item.img_id}
                            ></ImageWithPopover>
                            <HightLightMarkdown>
                              {item.highlight}
                            </HightLightMarkdown>
                          </Space>
                        </Card>
                      </List.Item>
                    )}
                  />
                )}
                {relatedQuestions?.length > 0 && (
                  <Card>
                    <Flex wrap="wrap" gap={'10px 0'}>
                      {relatedQuestions?.map((x, idx) => (
                        <Tag
                          key={idx}
                          className={styles.tag}
                          onClick={handleClickRelatedQuestion(x)}
                        >
                          {x}
                        </Tag>
                      ))}
                    </Flex>
                  </Card>
                )}
              </section>
              <section className={styles.graph}>
                {mindMapLoading ? (
                  <Skeleton active />
                ) : (
                  <IndentedTree
                    data={mindMap}
                    show
                    style={{ width: '100%', height: '100%' }}
                  ></IndentedTree>
                )}
              </section>
            </Flex>
          )}
        </Content>
      </Layout>
    </Layout>
  );
};

export default SearchPage;
