import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import HighLightMarkdown from '@/components/highlight-markdown';
import { FileIcon } from '@/components/icon-font';
import { ImageWithPopover } from '@/components/image';
import { Input } from '@/components/originui/input';
import { SkeletonCard } from '@/components/skeleton-card';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { IReference } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import DOMPurify from 'dompurify';
import { isEmpty } from 'lodash';
import { BrainCircuit, Search, X } from 'lucide-react';
import { Dispatch, SetStateAction, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ISearchAppDetailProps } from '../next-searches/hooks';
import PdfDrawer from './document-preview-modal';
import { ISearchReturnProps } from './hooks';
import './index.less';
import MarkdownContent from './markdown-content';
import MindMapDrawer from './mindmap-drawer';
import RetrievalDocuments from './retrieval-documents';
export default function SearchingView({
  setIsSearching,
  searchData,
  handleClickRelatedQuestion,
  handleTestChunk,
  setSelectedDocumentIds,
  answer,
  sendingLoading,
  relatedQuestions,
  isFirstRender,
  selectedDocumentIds,
  isSearchStrEmpty,
  searchStr,
  stopOutputMessage,
  visible,
  hideModal,
  documentId,
  selectedChunk,
  clickDocumentButton,
  mindMapVisible,
  hideMindMapModal,
  showMindMapModal,
  mindMapLoading,
  mindMap,
  chunks,
  total,
  handleSearch,
  pagination,
  onChange,
}: ISearchReturnProps & {
  setIsSearching?: Dispatch<SetStateAction<boolean>>;
  searchData: ISearchAppDetailProps;
}) {
  const { t } = useTranslation();
  // useEffect(() => {
  //   const changeLanguage = async () => {
  //     await i18n.changeLanguage('zh');
  //   };
  //   changeLanguage();
  // }, [i18n]);
  const [searchtext, setSearchtext] = useState<string>('');
  const [retrievalLoading, setRetrievalLoading] = useState(false);

  useEffect(() => {
    setSearchtext(searchStr);
  }, [searchStr, setSearchtext]);
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
            'text-4xl font-bold bg-gradient-to-l from-[#40EBE3] to-[#4A51FF] bg-clip-text cursor-pointer',
          )}
          onClick={() => {
            setIsSearching?.(false);
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
                placeholder={t('search.searchGreeting')}
                className={cn(
                  'w-full rounded-full py-6 pl-4 !pr-[8rem] text-primary text-lg bg-bg-base',
                )}
                value={searchtext}
                onChange={(e) => {
                  setSearchtext(e.target.value);
                }}
                disabled={sendingLoading}
                onKeyUp={(e) => {
                  if (e.key === 'Enter') {
                    handleSearch(searchtext);
                  }
                }}
              />
              <div className="absolute right-2 top-1/2 -translate-y-1/2 transform flex items-center gap-1">
                <X
                  className="text-text-secondary cursor-pointer opacity-80"
                  size={14}
                  onClick={() => {
                    setSearchtext('');
                    handleClickRelatedQuestion('');
                  }}
                />
                <span className="text-text-secondary opacity-20 ml-4">|</span>
                <button
                  type="button"
                  className="rounded-full bg-text-primary p-1 text-bg-base shadow w-12 h-8 ml-4"
                  onClick={() => {
                    if (sendingLoading) {
                      stopOutputMessage();
                    } else {
                      handleSearch(searchtext);
                    }
                  }}
                >
                  {sendingLoading ? (
                    // <Square size={22} className="m-auto" />
                    <div className="w-2 h-2 bg-bg-base m-auto"></div>
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
            {searchData.search_config.summary && !isSearchStrEmpty && (
              <>
                <div className="flex justify-start items-start text-text-primary text-2xl">
                  {t('search.AISummary')}
                </div>
                {isEmpty(answer) && sendingLoading ? (
                  <SkeletonCard className=" mt-2" />
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
                {answer.answer && !sendingLoading && (
                  <div className="w-full border-b border-border-default/80 my-6"></div>
                )}
              </>
            )}
            {/* retrieval documents */}
            {!isSearchStrEmpty && !sendingLoading && (
              <>
                <div className=" mt-3 w-44 ">
                  <RetrievalDocuments
                    selectedDocumentIds={selectedDocumentIds}
                    setSelectedDocumentIds={setSelectedDocumentIds}
                    onTesting={handleTestChunk}
                    setLoading={(loading: boolean) => {
                      setRetrievalLoading(loading);
                    }}
                  ></RetrievalDocuments>
                </div>
                {/* <div className="w-full border-b border-border-default/80 my-6"></div> */}
              </>
            )}
            <div className="mt-3 ">
              {chunks?.length > 0 && (
                <>
                  {chunks.map((chunk, index) => {
                    return (
                      <div key={index}>
                        <div className="w-full flex flex-col">
                          <div className="w-full highlightContent">
                            <ImageWithPopover
                              id={chunk.image_id || chunk.img_id}
                            ></ImageWithPopover>
                            <Popover>
                              <PopoverTrigger asChild>
                                <div
                                  dangerouslySetInnerHTML={{
                                    __html: DOMPurify.sanitize(
                                      `${
                                        chunk.highlight ??
                                        chunk.content_with_weight ??
                                        ''
                                      }...`,
                                    ),
                                  }}
                                  className="text-sm text-text-primary mb-1"
                                ></div>
                              </PopoverTrigger>
                              <PopoverContent className="text-text-primary !w-full max-w-lg ">
                                <div className="max-h-96 overflow-auto scrollbar-thin">
                                  <HighLightMarkdown>
                                    {chunk.content_with_weight}
                                  </HighLightMarkdown>
                                </div>
                              </PopoverContent>
                            </Popover>
                          </div>
                          <div
                            className="flex gap-2 items-center text-xs text-text-secondary border p-1 rounded-lg w-fit mt-3"
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
                      </div>
                    );
                  })}
                </>
              )}
              {relatedQuestions?.length > 0 &&
                searchData.search_config.related_search && (
                  <>
                    <div className="w-full border-b border-border-default/80 mt-6"></div>

                    <div className="mt-6 w-full overflow-hidden opacity-100 max-h-96">
                      <p className="text-text-primary mb-2 text-xl">
                        {t('search.relatedSearch')}
                      </p>
                      <div className="mt-2 flex flex-wrap justify-start gap-2">
                        {relatedQuestions?.map((x, idx) => (
                          <Button
                            key={idx}
                            variant="transparent"
                            className="bg-bg-card text-text-secondary"
                            onClick={handleClickRelatedQuestion(
                              x,
                              searchData.search_config.summary,
                            )}
                          >
                            {x}
                          </Button>
                        ))}
                      </div>
                    </div>
                  </>
                )}
            </div>
            {!isSearchStrEmpty &&
              !retrievalLoading &&
              !answer.answer &&
              !sendingLoading &&
              total <= 0 &&
              chunks?.length <= 0 &&
              relatedQuestions?.length <= 0 && (
                <div className="h-2/5 flex items-center justify-center">
                  <Empty type={EmptyType.SearchData} iconWidth={80} />
                </div>
              )}
          </div>

          {total > 0 && (
            <div className="mt-8 px-8 pb-8 text-base">
              <RAGFlowPagination
                current={pagination.current}
                pageSize={pagination.pageSize}
                total={total}
                onChange={onChange}
              ></RAGFlowPagination>
            </div>
          )}
        </div>
        {mindMapVisible && (
          <div className="flex-1 h-[88dvh] z-30 ml-32 mt-5">
            <MindMapDrawer
              visible={mindMapVisible}
              hideModal={hideMindMapModal}
              data={mindMap}
              loading={mindMapLoading}
            ></MindMapDrawer>
          </div>
        )}
      </div>
      {!mindMapVisible &&
        !isFirstRender &&
        !isSearchStrEmpty &&
        !isEmpty(searchData.search_config.kb_ids) &&
        searchData.search_config.query_mindmap && (
          <Popover>
            <PopoverTrigger asChild>
              <div
                className="rounded-lg h-16 w-16 p-0 absolute top-28 right-3 z-30 border cursor-pointer flex justify-center items-center bg-bg-card"
                onClick={showMindMapModal}
              >
                {/* <SvgIcon name="paper-clip" width={24} height={30}></SvgIcon> */}
                <BrainCircuit size={36} />
              </div>
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
    </section>
  );
}
