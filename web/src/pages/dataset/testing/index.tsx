import { Button } from '@/components/ui/button';
import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { Plus } from 'lucide-react';
import { useCallback, useState } from 'react';
import { TopTitle } from '../dataset-title';
import TestingForm from './testing-form';
import { TestingResult } from './testing-result';

function Vertical() {
  return <div>xxx</div>;
}

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

  const [count, setCount] = useState(1);

  const addCount = useCallback(() => {
    setCount(2);
  }, []);

  const removeCount = useCallback(() => {
    setCount(1);
  }, []);

  return (
    <div className="p-5">
      <section className="flex justify-between items-center">
        <TopTitle
          title={'Configuration'}
          description={`  Update your knowledge base configuration here, particularly the chunk
                  method.`}
        ></TopTitle>
        <Button>Save as Preset</Button>
      </section>
      {count === 1 ? (
        <section className="flex divide-x h-full">
          <div className="p-4 flex-1">
            <div className="flex justify-between pb-2.5">
              <span className="text-text-title font-semibold text-2xl">
                Test setting
              </span>
              <Button variant={'outline'} onClick={addCount}>
                <Plus /> Add New Test
              </Button>
            </div>
            <TestingForm
              loading={loading}
              setValues={setValues}
              refetch={refetch}
            ></TestingForm>
          </div>
          <TestingResult
            data={data}
            page={page}
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
