import ListFilterBar from '@/components/list-filter-bar';
import { t } from 'i18next';
import { useFetchMemoryMessageList, useSelectFilters } from './hook';
import { MemoryTable } from './message-table';

export default function MemoryMessage() {
  const {
    searchString,
    // documents,
    data,
    pagination,
    handleInputChange,
    setPagination,
    filterValue,
    handleFilterSubmit,
    loading,
  } = useFetchMemoryMessageList();
  const { filters } = useSelectFilters();
  return (
    <div className="flex flex-col gap-2">
      <ListFilterBar
        title="Dataset"
        onSearchChange={handleInputChange}
        searchString={searchString}
        // showFilter={false}
        // value={filterValue}
        // onChange={handleFilterSubmit}
        // onOpenChange={onOpenChange}
        // filters={filters}
        filters={filters}
        onChange={handleFilterSubmit}
        value={filterValue}
        leftPanel={
          <div className="items-start">
            <div className="pb-1">{t('memory.sideBar.messages')}</div>
            <div className="text-text-secondary text-sm font-normal">
              {t('memory.messages.messageDescription')}
            </div>
          </div>
        }
      ></ListFilterBar>
      <MemoryTable
        messages={data?.messages?.message_list ?? []}
        pagination={pagination}
        setPagination={setPagination}
        total={data?.messages?.total_count ?? 0}
        // rowSelection={rowSelection}
        // setRowSelection={setRowSelection}
        // loading={loading}
      ></MemoryTable>
    </div>
  );
}
