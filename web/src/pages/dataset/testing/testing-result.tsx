import { FormContainer } from '@/components/form-container';
import { FilterButton } from '@/components/list-filter-bar';
import { FilterPopover } from '@/components/list-filter-bar/filter-popover';
import { FilterCollection } from '@/components/list-filter-bar/interface';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { ITestingChunk } from '@/interfaces/database/knowledge';
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
>;

export function TestingResult({
  filterValue,
  handleFilterSubmit,
  page,
  pageSize,
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
        <span className="text-text-title font-semibold text-2xl">
          Test results
        </span>
        <FilterPopover
          filters={filters}
          onChange={handleFilterSubmit}
          value={filterValue}
        >
          <FilterButton></FilterButton>
        </FilterPopover>
      </div>
      <section className="flex flex-col gap-5 overflow-auto h-[76vh] mb-5">
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
    </div>
  );
}
