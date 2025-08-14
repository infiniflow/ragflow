import { FileIcon } from '@/components/icon-font';
import { ImageWithPopover } from '@/components/image';
import { Input } from '@/components/originui/input';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Skeleton } from '@/components/ui/skeleton';
import { Spin } from '@/components/ui/spin';
import { useSelectTestingResult } from '@/hooks/knowledge-hooks';
import { useGetPaginationWithRouter } from '@/hooks/logic-hooks';
import { IReference } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import DOMPurify from 'dompurify';
import { t } from 'i18next';
import { isEmpty } from 'lodash';
import { BrainCircuit, Search, Square, Tag, X } from 'lucide-react';
import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useRef,
} from 'react';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import { useSendQuestion, useShowMindMapDrawer } from '../search/hooks';
import PdfDrawer from './document-preview-modal';
import HightLightMarkdown from './highlight-markdown';
import './index.less';
import styles from './index.less';
import MarkdownContent from './markdown-content';
import MindMapDrawer from './mindmap-drawer';
import RetrievalDocuments from './retrieval-documents';
export default function SearchingPage({
  searchText,
  data: searchData,
  setIsSearching,
}: {
  searchText: string;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
  setSearchText: Dispatch<SetStateAction<string>>;
  data: ISearchAppDetailProps;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
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
    setSearchStr,
    stopOutputMessage,
  } = useSendQuestion(searchData.search_config.kb_ids);
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  useEffect(() => {
    if (searchText) {
      setSearchStr(searchText);
      sendQuestion(searchText);
    }
    // regain focus
    if (inputRef.current) {
      inputRef.current.focus();
    }
  }, [searchText, sendQuestion, setSearchStr]);

  const {
    mindMapVisible,
    hideMindMapModal,
    showMindMapModal,
    mindMapLoading,
    mindMap,
  } = useShowMindMapDrawer(searchData.search_config.kb_ids, searchStr);
  const { chunks, total } = useSelectTestingResult();
  const handleSearch = useCallback(() => {
    sendQuestion(searchStr);
  }, [searchStr, sendQuestion]);

  const { pagination, setPagination } = useGetPaginationWithRouter();
  const onChange = (pageNumber: number, pageSize: number) => {
    setPagination({ page: pageNumber, pageSize });
    handleTestChunk(selectedDocumentIds, pageNumber, pageSize);
  };

  return (
    <section
      className={cn(
        'relative w-full flex transition-all justify-start items-center',
      )}
    >
      {/* search header */}
      <div
        className={cn(
          'relative z-10 px-8 pt-8 flex  text-transparent justify-start items-start w-full',
        )}
      >
        <h1
          className={cn(
            'text-4xl font-bold bg-gradient-to-r from-sky-600 from-30% via-sky-500 via-60% to-emerald-500 bg-clip-text cursor-pointer',
          )}
          onClick={() => {
            setIsSearching(false);
          }}
        >
          RAGFlow
        </h1>

        <div
          className={cn(
            ' rounded-lg text-primary text-xl sticky flex flex-col justify-center w-2/3 max-w-[780px] transform scale-100 ml-16 ',
          )}
        >
          <div className={cn('flex flex-col justify-start items-start w-full')}>
            <div className="relative w-full text-primary">
              <Input
                ref={inputRef}
                key="search-input"
                placeholder="How can I help you today?"
                className={cn(
                  'w-full rounded-full py-6 pl-4 !pr-[8rem] text-primary text-lg bg-background',
                )}
                value={searchStr}
                onChange={handleSearchStrChange}
                disabled={sendingLoading}
                onKeyUp={(e) => {
                  if (e.key === 'Enter') {
                    handleSearch();
                  }
                }}
              />
              <div className="absolute right-2 top-1/2 -translate-y-1/2 transform flex items-center gap-1">
                <X
                  className="text-text-secondary"
                  size={14}
                  onClick={() => {
                    handleClickRelatedQuestion('');
                  }}
                />
                <span className="text-text-secondary">|</span>
                <button
                  type="button"
                  className="rounded-full bg-white p-1 text-gray-800 shadow w-12 h-8 ml-4"
                  onClick={() => {
                    if (sendingLoading) {
                      stopOutputMessage();
                    } else {
                      handleSearch();
                    }
                  }}
                >
                  {sendingLoading ? (
                    <Square size={22} className="m-auto" />
                  ) : (
                    <Search size={22} className="m-auto" />
                  )}
                </button>
              </div>
            </div>
          </div>

          {/* search body */}
          <div
            className="w-full mt-5 overflow-auto scrollbar-none "
            style={{ height: 'calc(100vh - 250px)' }}
          >
            {searchData.search_config.summary && (
              <>
                <div className="flex justify-start items-start text-text-primary text-2xl">
                  AI Summary
                </div>
                {isEmpty(answer) && sendingLoading ? (
                  <div className="space-y-2">
                    <Skeleton className="h-4 w-full bg-bg-card" />
                    <Skeleton className="h-4 w-full bg-bg-card" />
                    <Skeleton className="h-4 w-2/3 bg-bg-card" />
                  </div>
                ) : (
                  answer.answer && (
                    <div className="border rounded-lg p-4 mt-3 max-h-52 overflow-auto scrollbar-none">
                      <MarkdownContent
                        loading={sendingLoading}
                        content={answer.answer}
                        reference={answer.reference ?? ({} as IReference)}
                        clickDocumentButton={clickDocumentButton}
                      ></MarkdownContent>
                    </div>
                  )
                )}
              </>
            )}

            <div className="w-full border-b border-border-default/80 my-6"></div>
            {/* retrieval documents */}
            <div className=" mt-3 w-44 ">
              <RetrievalDocuments
                selectedDocumentIds={selectedDocumentIds}
                setSelectedDocumentIds={setSelectedDocumentIds}
                onTesting={handleTestChunk}
              ></RetrievalDocuments>
            </div>
            <div className="w-full border-b border-border-default/80 my-6"></div>
            <div className="mt-3 ">
              <Spin spinning={loading}>
                {chunks?.length > 0 && (
                  <>
                    {chunks.map((chunk, index) => {
                      return (
                        <>
                          <div
                            key={chunk.chunk_id}
                            className="w-full flex flex-col"
                          >
                            <div className="w-full">
                              <ImageWithPopover
                                id={chunk.img_id}
                              ></ImageWithPopover>
                              <Popover>
                                <PopoverTrigger asChild>
                                  <div
                                    dangerouslySetInnerHTML={{
                                      __html: DOMPurify.sanitize(
                                        `${chunk.highlight}...`,
                                      ),
                                    }}
                                    className="text-sm text-text-primary mb-1"
                                  ></div>
                                </PopoverTrigger>
                                <PopoverContent className="text-text-primary">
                                  <HightLightMarkdown>
                                    {chunk.content_with_weight}
                                  </HightLightMarkdown>
                                </PopoverContent>
                              </Popover>
                            </div>
                            <div
                              className="flex gap-2 items-center text-xs text-text-secondary border p-1 rounded-lg w-fit"
                              onClick={() =>
                                clickDocumentButton(chunk.doc_id, chunk as any)
                              }
                            >
                              <FileIcon name={chunk.docnm_kwd}></FileIcon>
                              {chunk.docnm_kwd}
                            </div>
                          </div>
                          {index < chunks.length - 1 && (
                            <div className="w-full border-b border-border-default/80 mt-6"></div>
                          )}
                        </>
                      );
                    })}
                  </>
                )}
              </Spin>
              {relatedQuestions?.length > 0 && (
                <div title={t('chat.relatedQuestion')}>
                  <div className="flex gap-2">
                    {relatedQuestions?.map((x, idx) => (
                      <Tag
                        key={idx}
                        className={styles.tag}
                        onClick={handleClickRelatedQuestion(x)}
                      >
                        {x}
                      </Tag>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
          <div className="mt-8 px-8 pb-8">
            <RAGFlowPagination
              current={pagination.current}
              pageSize={pagination.pageSize}
              total={total}
              onChange={onChange}
            ></RAGFlowPagination>
          </div>
        </div>
      </div>
      {!mindMapVisible &&
        !isFirstRender &&
        !isSearchStrEmpty &&
        !isEmpty(searchData.search_config.kb_ids) && (
          <Popover>
            <PopoverTrigger asChild>
              <Button
                className="rounded-lg h-8 w-8 p-0 absolute top-28 right-3 z-30"
                variant={'transparent'}
                onClick={showMindMapModal}
              >
                {/* <SvgIcon name="paper-clip" width={24} height={30}></SvgIcon> */}
                <BrainCircuit size={24} />
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-fit">{t('chunk.mind')}</PopoverContent>
          </Popover>
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
        <div className="absolute top-20 right-16 z-30">
          <MindMapDrawer
            visible={mindMapVisible}
            hideModal={hideMindMapModal}
            data={mindMap}
            loading={mindMapLoading}
          ></MindMapDrawer>
        </div>
      )}
    </section>
  );
}
