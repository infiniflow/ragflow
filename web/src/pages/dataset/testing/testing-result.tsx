import Empty from '@/components/empty/empty';
import { FormContainer } from '@/components/form-container';
import { FilterButton } from '@/components/list-filter-bar';
import { FilterPopover } from '@/components/list-filter-bar/filter-popover';
import { FilterCollection } from '@/components/list-filter-bar/interface';
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
    <div className="flex gap-3 text-xs text-text-sub-title-invert italic">
      {similarityList.map((x) => (
        <div key={x.field} className="space-x-1">
          <span>{((item[x.field] as number) * 100).toFixed(2)}</span>
          <span>{t(camelCase(x.field))}</span>
        </div>
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
    <div className="p-4 flex-1">
      <div className="flex justify-between pb-2.5">
        <span className="text-text-primary font-semibold text-2xl">
          {t('knowledgeDetails.testResults')}
        </span>
        <FilterPopover
          filters={filters}
          onChange={handleFilterSubmit}
          value={filterValue}
        >
          <FilterButton></FilterButton>
        </FilterPopover>
      </div>
      {data.chunks?.length > 0 && !loading && (
        <>
          <section className="flex flex-col gap-5 overflow-auto h-[calc(100vh-241px)] scrollbar-thin mb-5">
            {data.chunks?.map((x) => (
              <FormContainer key={x.chunk_id} className="px-5 py-2.5">
                <ChunkTitle item={x}></ChunkTitle>
                <p className="!mt-2.5"> {x.content_with_weight}</p>
              </FormContainer>
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
        <div className="flex justify-center items-center w-full h-[calc(100vh-241px)]">
          <div>
            <Empty>
              {data.isRuned && (
                <div className="text-text-secondary">
                  {t('knowledgeDetails.noTestResultsForRuned')}
                </div>
              )}
              {!data.isRuned && (
                <div className="text-text-secondary">
                  {t('knowledgeDetails.noTestResultsForNotRuned')}
                </div>
              )}
            </Empty>
          </div>
        </div>
      )}
    </div>
  );
}
