import FileIcon from '@/components/file-icon';
import HightLightMarkdown from '@/components/highlight-markdown';
import { ImageWithPopover } from '@/components/image';
import PdfDrawer from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import RetrievalDocuments from '@/components/retrieval-documents';
import SvgIcon from '@/components/svg-icon';
import {
  useFetchKnowledgeList,
  useSelectTestingResult,
} from '@/hooks/knowledge-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IReference } from '@/interfaces/database/chat';
import {
  Button,
  Card,
  Divider,
  Flex,
  FloatButton,
  Input,
  Layout,
  List,
  Pagination,
  PaginationProps,
  Popover,
  Skeleton,
  Space,
  Spin,
  Tag,
  Tooltip,
} from 'antd';
import classNames from 'classnames';
import DOMPurify from 'dompurify';
import { isEmpty } from 'lodash';
import { CircleStop, SendHorizontal } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import MarkdownContent from '../chat/markdown-content';
import { useSendQuestion, useShowMindMapDrawer } from './hooks';
import styles from './index.less';
import MindMapDrawer from './mindmap-drawer';
import SearchSidebar from './sidebar';

const { Content } = Layout;
const { Search } = Input;

const SearchPage = () => {
  const { t } = useTranslation();
  const [checkedList, setCheckedList] = useState<string[]>([]);
  const { chunks, total } = useSelectTestingResult();
  const { list: knowledgeList } = useFetchKnowledgeList();
  const checkedWithoutEmbeddingIdList = useMemo(() => {
    return checkedList.filter((x) => knowledgeList.some((y) => y.id === x));
  }, [checkedList, knowledgeList]);

  const {
    sendQuestion,
    handleClickRelatedQuestion,
    handleSearchStrChange,
    handleTestChunk,
    setSelectedDocumentIds,
    answer,
    sendingLoading,
    relatedQuestions,
    searchStr,
    loading,
    isFirstRender,
    selectedDocumentIds,
    isSearchStrEmpty,
    stopOutputMessage,
  } = useSendQuestion(checkedWithoutEmbeddingIdList);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();
  const { pagination } = useGetPaginationWithRouter();
  const {
    mindMapVisible,
    hideMindMapModal,
    showMindMapModal,
    mindMapLoading,
    mindMap,
  } = useShowMindMapDrawer(checkedWithoutEmbeddingIdList, searchStr);

  const onChange: PaginationProps['onChange'] = (pageNumber, pageSize) => {
    pagination.onChange?.(pageNumber, pageSize);
    handleTestChunk(selectedDocumentIds, pageNumber, pageSize);
  };

  const handleSearch = useCallback(() => {
    sendQuestion(searchStr);
  }, [searchStr, sendQuestion]);

  const InputSearch = (
    <Search
      value={searchStr}
      onChange={handleSearchStrChange}
      placeholder={t('header.search')}
      allowClear
      addonAfter={
        sendingLoading ? (
          <Button onClick={stopOutputMessage}>
            <CircleStop />
          </Button>
        ) : (
          <Button onClick={handleSearch}>
            <SendHorizontal className="size-5 text-blue-500" />
          </Button>
        )
      }
      onSearch={sendQuestion}
      size="large"
      loading={sendingLoading}
      disabled={checkedWithoutEmbeddingIdList.length === 0}
      className={classNames(
        styles.searchInput,
        isFirstRender ? styles.globalInput : styles.partialInput,
      )}
    />
  );

  return (
    <>
      <Layout className={styles.searchPage}>
        <SearchSidebar
          isFirstRender={isFirstRender}
          checkedList={checkedWithoutEmbeddingIdList}
          setCheckedList={setCheckedList}
        ></SearchSidebar>
        <Layout className={isFirstRender ? styles.mainLayout : ''}>
          <Content>
            {isFirstRender ? (
              <Flex justify="center" className={styles.firstRenderContent}>
                <Flex vertical align="center" gap={'large'}>
                  {InputSearch}
                </Flex>
              </Flex>
            ) : (
              <Flex className={styles.content}>
                <section className={styles.main}>
                  {InputSearch}
                  <Card
                    title={
                      <Flex gap={10}>
                        <img src="/logo.svg" alt="" width={20} />
                        {t('chat.answerTitle')}
                      </Flex>
                    }
                    className={styles.answerWrapper}
                  >
                    {isEmpty(answer) && sendingLoading ? (
                      <Skeleton active />
                    ) : (
                      answer.answer && (
                        <MarkdownContent
                          loading={sendingLoading}
                          content={answer.answer}
                          reference={answer.reference ?? ({} as IReference)}
                          clickDocumentButton={clickDocumentButton}
                        ></MarkdownContent>
                      )
                    )}
                  </Card>
                  <Divider></Divider>
                  <RetrievalDocuments
                    selectedDocumentIds={selectedDocumentIds}
                    setSelectedDocumentIds={setSelectedDocumentIds}
                    onTesting={handleTestChunk}
                  ></RetrievalDocuments>
                  <Divider></Divider>
                  <Spin spinning={loading}>
                    {chunks?.length > 0 && (
                      <List
                        dataSource={chunks || []}
                        className={styles.chunks}
                        renderItem={(item) => (
                          <List.Item>
                            <Card className={styles.card}>
                              <Space>
                                <ImageWithPopover
                                  id={item.img_id}
                                ></ImageWithPopover>
                                <Flex vertical gap={10}>
                                  <Popover
                                    content={
                                      <div className={styles.popupMarkdown}>
                                        <HightLightMarkdown>
                                          {item.content_with_weight}
                                        </HightLightMarkdown>
                                      </div>
                                    }
                                  >
                                    <div
                                      dangerouslySetInnerHTML={{
                                        __html: DOMPurify.sanitize(
                                          `${item.highlight}...`,
                                        ),
                                      }}
                                      className={styles.highlightContent}
                                    ></div>
                                  </Popover>
                                  <Space
                                    className={styles.documentReference}
                                    onClick={() =>
                                      clickDocumentButton(
                                        item.doc_id,
                                        item as any,
                                      )
                                    }
                                  >
                                    <FileIcon
                                      id={item.image_id}
                                      name={item.docnm_kwd}
                                    ></FileIcon>
                                    {item.docnm_kwd}
                                  </Space>
                                </Flex>
                              </Space>
                            </Card>
                          </List.Item>
                        )}
                      />
                    )}
                  </Spin>
                  {relatedQuestions?.length > 0 && (
                    <Card title={t('chat.relatedQuestion')}>
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
                    className={styles.pagination}
                  />
                </section>
              </Flex>
            )}
          </Content>
        </Layout>
      </Layout>
      {!isFirstRender &&
        !isSearchStrEmpty &&
        !isEmpty(checkedWithoutEmbeddingIdList) && (
          <Tooltip title={t('chunk.mind')} zIndex={1}>
            <FloatButton
              className={styles.mindMapFloatButton}
              onClick={showMindMapModal}
              icon={
                <SvgIcon name="paper-clip" width={24} height={30}></SvgIcon>
              }
            />
          </Tooltip>
        )}
      {visible && (
        <PdfDrawer
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={selectedChunk}
        ></PdfDrawer>
      )}
      {mindMapVisible && (
        <MindMapDrawer
          visible={mindMapVisible}
          hideModal={hideMindMapModal}
          data={mindMap}
          loading={mindMapLoading}
        ></MindMapDrawer>
      )}
    </>
  );
};

export default SearchPage;
