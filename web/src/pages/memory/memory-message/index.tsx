import ListFilterBar from '@/components/list-filter-bar';
import { t } from 'i18next';
import { useFetchMemoryMessageList } from './hook';
import { MemoryTable } from './message-table';
import { useFetchMemoryBaseConfiguration } from '../hooks/use-memory-setting';
import { useCanWriteMemoryRole } from '@/ee/pages/memories/memory-dropdown-guard';

export default function MemoryMessage() {
  const { data: memory } = useFetchMemoryBaseConfiguration();
  const hasRoleWrite = useCanWriteMemoryRole(memory?.tenant_id ?? '');
  const {
    searchString,
    // documents,
    data,
    pagination,
    handleInputChange,
    setPagination,
    loading,
  } = useFetchMemoryMessageList();
  return (
    <div className="flex flex-col gap-2">
      <ListFilterBar
        title={t('header.dataset')}
        onSearchChange={handleInputChange}
        searchString={searchString}
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
        operatorPermission={memory?.operator_permission ?? 0}
        hasRoleWrite={hasRoleWrite}
        // rowSelection={rowSelection}
        // setRowSelection={setRowSelection}
        // loading={loading}
      ></MemoryTable>
    </div>
  );
}
