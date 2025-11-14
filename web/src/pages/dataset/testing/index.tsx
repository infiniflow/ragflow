import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { t } from 'i18next';
import { useState } from 'react';
import { TopTitle } from '../dataset-title';
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
    <div className="p-5">
      <section className="flex justify-between items-center">
        <TopTitle
          title={t('knowledgeDetails.retrievalTesting')}
          description={t('knowledgeDetails.testingDescription')}
        ></TopTitle>
        {/* <Button>Save as Preset</Button> */}
      </section>
      {count === 1 ? (
        <section className="flex divide-x h-full">
          <div className="p-4 flex-1">
            <div className="flex justify-between pb-2.5">
              <span className="text-text-primary font-semibold text-2xl">
                {t('knowledgeDetails.testSetting')}
              </span>
              {/* <Button variant={'outline'} onClick={addCount}>
                <Plus /> Add New Test
              </Button> */}
            </div>
            <div className="h-[calc(100vh-241px)] overflow-auto scrollbar-thin">
              <TestingForm
                loading={loading}
                setValues={setValues}
                refetch={refetch}
              ></TestingForm>
            </div>
          </div>
          <TestingResult
            data={data}
            page={page}
            loading={loading}
            pageSize={pageSize}
            filterValue={filterValue}
            handleFilterSubmit={handleFilterSubmit}
            onPaginationChange={onPaginationChange}
          ></TestingResult>
        </section>
      ) : (
        <section className="flex gap-2">
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
        </section>
      )}
    </div>
  );
}
