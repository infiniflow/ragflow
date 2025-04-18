import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useTranslate } from '@/hooks/common-hooks';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { ITestingChunk } from '@/interfaces/database/knowledge';
import { camelCase } from 'lodash';
import TestingForm from './testing-form';

const similarityList: Array<{ field: keyof ITestingChunk; label: string }> = [
  { field: 'similarity', label: 'Hybrid Similarity' },
  { field: 'term_similarity', label: 'Term Similarity' },
  { field: 'vector_similarity', label: 'Vector Similarity' },
];

const ChunkTitle = ({ item }: { item: ITestingChunk }) => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <div className="flex gap-3 text-xs">
      {similarityList.map((x) => (
        <div key={x.field} className="space-x-1">
          <span>{((item[x.field] as number) * 100).toFixed(2)}</span>
          <span>{t(camelCase(x.field))}</span>
        </div>
      ))}
    </div>
  );
};

export default function RetrievalTesting() {
  const { loading, setValues, refetch, data } = useTestRetrieval();

  return (
    <section className="flex divide-x h-full">
      <div className="p-4">
        <TestingForm
          loading={loading}
          setValues={setValues}
          refetch={refetch}
        ></TestingForm>
      </div>
      <div className="p-4 flex-1 ">
        <h2 className="text-4xl font-bold mb-8 px-[10%]">
          15 Results from 3 files
        </h2>
        <section className="flex flex-col gap-4 overflow-auto h-[83vh] px-[10%]">
          {data.chunks.map((x) => (
            <Card
              key={x.chunk_id}
              className="bg-colors-background-neutral-weak border-colors-outline-neutral-strong"
            >
              <CardHeader>
                <CardTitle>
                  <div className="flex gap-2 flex-wrap">
                    <ChunkTitle item={x}></ChunkTitle>
                  </div>
                </CardTitle>
              </CardHeader>
              <CardContent>
                <p className="text-colors-text-neutral-strong">
                  {x.content_with_weight}
                </p>
              </CardContent>
            </Card>
          ))}
        </section>
      </div>
    </section>
  );
}
