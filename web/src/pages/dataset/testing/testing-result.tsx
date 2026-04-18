import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import HighLightMarkdown from '@/components/highlight-markdown';
import { FilterButton } from '@/components/list-filter-bar';
import { FilterPopover } from '@/components/list-filter-bar/filter-popover';
import { FilterCollection } from '@/components/list-filter-bar/interface';
import { Card } from '@/components/ui/card';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { ITestingChunk } from '@/interfaces/database/knowledge';
import { t } from 'i18next';
import camelCase from 'lodash/camelCase';
import { useMemo } from 'react';

const similarityList: Array<{ field: keyof ITestingChunk; label: string }> = [
  { field: 'similarity', label: 'Hybrid Similarity' },
  { field: 'term_similarity', label: 'Term Similarity' },
  { field: 'vector_similarity', label: 'Vector Similarity' },
];

const ChunkTitle = ({ item }: { item: ITestingChunk }) => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <div className="text-xs text-text-sub-title-invert italic space-x-4 rtl:space-x-reverse">
      {similarityList.map((x) => (
        <p key={x.field} className="inline">
          {((item[x.field] as number) * 100).toFixed(2)}{' '}
          <dfn>{t(camelCase(x.field))}</dfn>
        </p>
      ))}
    </div>
  );
};

type TestingResultProps = Pick<
  ReturnType<typeof useTestRetrieval>,
  | 'data'
  | 'filterValue'
  | 'handleFilterSubmit'
  | 'page'
  | 'pageSize'
  | 'onPaginationChange'
  | 'loading'
>;

export function TestingResult({
  filterValue,
  handleFilterSubmit,
  page,
  pageSize,
  loading,
  onPaginationChange,
  data,
}: TestingResultProps) {
  const filters: FilterCollection[] = useMemo(() => {
    return [
      {
        field: 'doc_ids',
        label: 'File',
        list:
          data.doc_aggs?.map((x) => ({
            id: x.doc_id,
            label: x.doc_name,
            count: x.count,
          })) ?? [],
      },
    ];
  }, [data.doc_aggs]);

  return (
    <article className="size-full flex flex-col">
      <header className="flex-0 px-5 py-3 flex justify-between">
        <h2 className="font-semibold text-base leading-8">
          {t('knowledgeDetails.testResults')}
        </h2>

        <FilterPopover
          filters={filters}
          onChange={handleFilterSubmit}
          value={filterValue}
        >
          <FilterButton></FilterButton>
        </FilterPopover>
      </header>

      <div className="flex-1 h-0">
        {data.chunks?.length > 0 && !loading && (
          <>
            <section className="px-5 pb-5 flex flex-col gap-5 overflow-auto h-full scrollbar-thin">
              {data.chunks?.map((x) => (
                <article key={x.chunk_id}>
                  <Card className="px-5 py-2.5 bg-transparent shadow-none">
                    <ChunkTitle item={x}></ChunkTitle>
                    <div className="!mt-2.5">
                      <HighLightMarkdown>
                        {x.highlight || x.content_with_weight}
                      </HighLightMarkdown>
                    </div>
                  </Card>
                </article>
              ))}
            </section>
            <RAGFlowPagination
              total={data.total}
              onChange={onPaginationChange}
              current={page}
              pageSize={pageSize}
            ></RAGFlowPagination>
          </>
        )}
        {!data.chunks?.length && !loading && (
          <div className="size-full p-5 flex justify-center items-center">
            <div>
              <Empty type={EmptyType.SearchData} iconWidth={80}>
                <div className="text-text-secondary text-sm">
                  {t(
                    data.isRuned
                      ? 'knowledgeDetails.noTestResultsForRuned'
                      : 'knowledgeDetails.noTestResultsForNotRuned',
                  )}
                </div>
              </Empty>
            </div>
          </div>
        )}
      </div>
    </article>
  );
}
