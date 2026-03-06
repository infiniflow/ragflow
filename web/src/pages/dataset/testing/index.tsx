import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { t } from 'i18next';
import { useState } from 'react';
import TestingForm from './testing-form';
import { TestingResult } from './testing-result';

export default function RetrievalTesting() {
  const {
    loading,
    setValues,
    refetch,
    data,
    onPaginationChange,
    page,
    pageSize,
    handleFilterSubmit,
    filterValue,
  } = useTestRetrieval();

  const [count] = useState(1);

  return (
    <div className="pr-5 pb-5">
      <Card className="size-full bg-transparent shadow-none flex flex-col">
        <CardHeader className="p-5 border-b-0.5 border-border-button">
          <header>
            <CardTitle as="h1">
              {t('knowledgeDetails.retrievalTesting')}
            </CardTitle>

            <CardDescription>
              {t('knowledgeDetails.testingDescription')}
            </CardDescription>

            {/* <Button>Save as Preset</Button> */}
          </header>
        </CardHeader>

        {count === 1 ? (
          <CardContent className="flex-1 overflow-hidden p-0 grid grid-rows-1 grid-cols-2 divide-x-0.5">
            <article className="size-full flex-1 flex flex-col">
              <header className="px-5 py-3">
                <h2 className="font-semibold text-base leading-8">
                  {t('knowledgeDetails.testSetting')}
                </h2>
                {/* <Button variant={'outline'} onClick={addCount}>
                  <Plus /> Add New Test
                </Button> */}
              </header>

              <div className="flex-1 h-0">
                <TestingForm
                  loading={loading}
                  setValues={setValues}
                  refetch={refetch}
                />
              </div>
            </article>

            <div className="flex-1">
              <TestingResult
                data={data}
                page={page}
                loading={loading}
                pageSize={pageSize}
                filterValue={filterValue}
                handleFilterSubmit={handleFilterSubmit}
                onPaginationChange={onPaginationChange}
              />
            </div>
          </CardContent>
        ) : (
          <CardContent className="p-0 flex gap-2">
            <div className="flex-1">
              <TestingForm
                loading={loading}
                setValues={setValues}
                refetch={refetch}
              ></TestingForm>
              <TestingResult
                data={data}
                page={page}
                loading={loading}
                pageSize={pageSize}
                filterValue={filterValue}
                handleFilterSubmit={handleFilterSubmit}
                onPaginationChange={onPaginationChange}
              ></TestingResult>
            </div>
            <div className="flex-1">
              <TestingForm
                loading={loading}
                setValues={setValues}
                refetch={refetch}
              ></TestingForm>
              <TestingResult
                data={data}
                page={page}
                loading={loading}
                pageSize={pageSize}
                filterValue={filterValue}
                handleFilterSubmit={handleFilterSubmit}
                onPaginationChange={onPaginationChange}
              ></TestingResult>
            </div>
          </CardContent>
        )}
      </Card>
    </div>
  );
}
