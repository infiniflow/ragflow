import HightLightMarkdown from '@/components/highlight-markdown';
import { ImageWithPopover } from '@/components/image';
import IndentedTree from '@/components/indented-tree/indented-tree';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import RetrievalDocuments from '@/components/retrieval-documents';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IReference } from '@/interfaces/database/chat';
import {
  Card,
  Divider,
  Flex,
  Input,
  Layout,
  List,
  Pagination,
  PaginationProps,
  Skeleton,
  Space,
  Tag,
} from 'antd';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import MarkdownContent from '../chat/markdown-content';
import { useFetchBackgroundImage, useSendQuestion } from './hooks';
import SearchSidebar from './sidebar';

import styles from './index.less';

const { Content } = Layout;
const { Search } = Input;

const SearchPage = () => {
  const { t } = useTranslation();
  const [checkedList, setCheckedList] = useState<string[]>([]);
  const { chunks, total } = useSelectTestingResult();
  // const appConf = useFetchAppConf();
  const {
    sendQuestion,
    handleClickRelatedQuestion,
    handleSearchStrChange,
    handleTestChunk,
    setSelectedDocumentIds,
    answer,
    sendingLoading,
    relatedQuestions,
    mindMap,
    mindMapLoading,
    searchStr,
    loading,
    isFirstRender,
    selectedDocumentIds,
  } = useSendQuestion(checkedList);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  const imgUrl = useFetchBackgroundImage();
  const { pagination } = useGetPaginationWithRouter();

  const onChange: PaginationProps['onChange'] = (pageNumber, pageSize) => {
    pagination.onChange?.(pageNumber, pageSize);
    handleTestChunk(selectedDocumentIds, pageNumber, pageSize);
  };

  const InputSearch = (
    <Search
      value={searchStr}
      onChange={handleSearchStrChange}
      placeholder={t('header.search')}
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
    <>
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
                style={{ backgroundImage: `url(${imgUrl})` }}
              >
                <Flex vertical align="center" gap={'large'}>
                  {/* <Space size={30}>
                    <img src="/logo.svg" alt="" className={styles.appIcon} />
                    <span className={styles.appName}>{appConf.appName}</span>
                  </Space> */}
                  {InputSearch}
                </Flex>
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
                        clickDocumentButton={clickDocumentButton}
                      ></MarkdownContent>
                    </div>
                  )}
                  <Divider></Divider>
                  <RetrievalDocuments
                    selectedDocumentIds={selectedDocumentIds}
                    setSelectedDocumentIds={setSelectedDocumentIds}
                    onTesting={handleTestChunk}
                  ></RetrievalDocuments>
                  <Divider></Divider>
                  {chunks.length > 0 && (
                    <List
                      dataSource={chunks}
                      loading={loading}
                      className={styles.chunks}
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
                  <Divider></Divider>
                  <Pagination
                    {...pagination}
                    total={total}
                    onChange={onChange}
                  />
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
      <PdfDrawer
        visible={visible}
        hideModal={hideModal}
        documentId={documentId}
        chunk={selectedChunk}
      ></PdfDrawer>
    </>
  );
};

export default SearchPage;
