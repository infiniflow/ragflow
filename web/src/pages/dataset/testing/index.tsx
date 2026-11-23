import {
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { PageFrame } from '@/components/page-frame';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { t } from 'i18next';
import TestingForm from './testing-form';
import { TestingResult } from './testing-result';

export default function RetrievalTesting() {
  const { loading, setValues, refetch, data, handleFilterSubmit, filterValue } =
    useTestRetrieval();

  return (
    <PageFrame>
      <CardHeader className="p-5 border-b-0.5 border-border-button shrink-0">
        <header>
          <CardTitle as="h1">
            {t('knowledgeDetails.retrievalTesting')}
          </CardTitle>

          <CardDescription>
            {t('knowledgeDetails.testingDescription')}
          </CardDescription>
        </header>
      </CardHeader>

      <CardContent className="flex-1 min-h-0 overflow-hidden p-0 grid grid-rows-1 grid-cols-2 divide-x-0.5">
        <article className="min-h-0 min-w-0 flex flex-col">
          <header className="px-5 py-3 shrink-0">
            <h2 className="font-semibold text-base leading-8">
              {t('knowledgeDetails.testSetting')}
            </h2>
          </header>

          <div className="flex-1 h-0 min-h-0">
            <TestingForm
              loading={loading}
              setValues={setValues}
              refetch={refetch}
            />
          </div>
        </article>

        <div className="min-h-0 min-w-0">
          <TestingResult
            data={data}
            loading={loading}
            filterValue={filterValue}
            handleFilterSubmit={handleFilterSubmit}
          />
        </div>
      </CardContent>
    </PageFrame>
  );
}
