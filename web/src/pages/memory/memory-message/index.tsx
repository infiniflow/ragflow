import ListFilterBar from '@/components/list-filter-bar';
import { t } from 'i18next';
import { useFetchMemoryMessageList } from './hook';
import { MemoryTable } from './message-table';

export default function MemoryMessage() {
  const {
    searchString,
    // documents,
    data,
    pagination,
    handleInputChange,
    setPagination,
    // filterValue,
    // handleFilterSubmit,
    loading,
  } = useFetchMemoryMessageList();
  return (
    <div className="flex flex-col gap-2">
      <ListFilterBar
        title="Dataset"
        onSearchChange={handleInputChange}
        searchString={searchString}
        // value={filterValue}
        // onChange={handleFilterSubmit}
        // onOpenChange={onOpenChange}
        // filters={filters}
        leftPanel={
          <div className="items-start">
            <div className="pb-1">{t('knowledgeDetails.subbarFiles')}</div>
            <div className="text-text-secondary text-sm">
              {t('knowledgeDetails.datasetDescription')}
            </div>
          </div>
        }
      ></ListFilterBar>
      <MemoryTable
        messages={data?.messages?.message_list ?? []}
        pagination={pagination}
        setPagination={setPagination}
        total={data?.messages?.total ?? 0}
        // rowSelection={rowSelection}
        // setRowSelection={setRowSelection}
        // loading={loading}
      ></MemoryTable>
    </div>
  );
}
