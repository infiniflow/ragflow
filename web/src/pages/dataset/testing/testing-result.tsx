import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { FilterButton } from '@/components/list-filter-bar';
import { FilterPopover } from '@/components/list-filter-bar/filter-popover';
import { FilterCollection } from '@/components/list-filter-bar/interface';
import { Card } from '@/components/ui/card';
import { useTranslate } from '@/hooks/common-hooks';
import { buildChunkParsedResultPath } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useKnowledgeBaseId,
  useTestRetrieval,
} from '@/hooks/use-knowledge-request';
import { ITestingChunk } from '@/interfaces/database/dataset';
import { sanitizeHtmlWithImagesAsText } from '@/utils/dom-util';
import { t } from 'i18next';
import camelCase from 'lodash/camelCase';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';

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

type ChunkResultCardProps = {
  item: ITestingChunk;
  knowledgeBaseId: string;
};

function ChunkResultCard({ item, knowledgeBaseId }: ChunkResultCardProps) {
  const { t } = useTranslation();

  return (
    <article>
      <Card className="px-5 py-2.5 bg-transparent shadow-none">
        <ChunkTitle item={item}></ChunkTitle>
        <div
          className="!mt-2.5 whitespace-pre-wrap [&_em]:text-accent-primary [&_em]:not-italic"
          dangerouslySetInnerHTML={{
            __html: sanitizeHtmlWithImagesAsText(
              item.highlight || item.content,
            ),
          }}
        />
        <div className="mt-2.5 text-right text-xs text-text-sub-title-invert">
          {/* A real link so the hit can be middle-clicked or opened in the
              background; it targets a new tab because the retrieval results
              only live in memory and are lost on a back navigation. */}
          <Link
            to={buildChunkParsedResultPath(
              item.document_id,
              item.dataset_id || knowledgeBaseId,
              item.id,
            )}
            target="_blank"
            rel="noreferrer"
            title={t('knowledgeDetails.openChunkInDocument')}
            className="rounded-sm underline underline-offset-2 hover:text-accent-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-primary"
          >
            {item.document_keyword}
          </Link>
        </div>
      </Card>
    </article>
  );
}

type TestingResultProps = Pick<
  ReturnType<typeof useTestRetrieval>,
  'data' | 'filterValue' | 'handleFilterSubmit' | 'loading'
>;

export function TestingResult({
  filterValue,
  handleFilterSubmit,
  loading,
  data,
}: TestingResultProps) {
  const knowledgeBaseId = useKnowledgeBaseId();

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
      <header className="flex-0 px-5 py-3 flex justify-between items-center">
        <h2 className="font-semibold text-base leading-8">
          {t('knowledgeDetails.testResults')}
        </h2>
        <span className="mr-auto text-sm text-text-secondary pl-2">
          {t('common.total')}: {data.total}
        </span>

        <FilterPopover
          filters={filters}
          onChange={handleFilterSubmit}
          value={filterValue}
        >
          <FilterButton></FilterButton>
        </FilterPopover>
      </header>

      <>
        {data.chunks?.length > 0 && !loading && (
          <>
            <section className="px-5 pb-5 flex flex-col gap-5 overflow-auto scrollbar-thin min-h-0">
              {data.chunks?.map((x) => (
                <ChunkResultCard
                  key={x.id}
                  item={x}
                  knowledgeBaseId={knowledgeBaseId}
                ></ChunkResultCard>
              ))}
            </section>
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
      </>
    </article>
  );
}
