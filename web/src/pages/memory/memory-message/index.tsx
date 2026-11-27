import ListFilterBar from '@/components/list-filter-bar';
import { CardContent, CardHeader } from '@/components/ui/card';
import { t } from 'i18next';
import { useFetchMemoryMessageList } from './hook';
import { MemoryTable } from './message-table';
import { useFetchMemoryBaseConfiguration } from '../hooks/use-memory-setting';
import { useCanWriteMemoryRole } from '@/ee/pages/memories/memory-dropdown-guard';
import { formatDate } from '@/utils/date';

export default function MemoryMessage() {
  const { data: memory } = useFetchMemoryBaseConfiguration();
  const hasRoleWrite = useCanWriteMemoryRole(memory?.tenant_id ?? '');
  const { searchString, data, pagination, handleInputChange, setPagination } =
    useFetchMemoryMessageList();
  return (
    <>
      <CardHeader as="header" className="p-5 space-y-0 shrink-0">
        <ListFilterBar
          className="items-end"
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
      </CardHeader>
      <CardContent className="px-5 py-0 flex-1 min-h-0 flex flex-col">
        <MemoryTable
          messages={data?.messages?.message_list ?? []}
          pagination={pagination}
          setPagination={setPagination}
          total={data?.messages?.total_count ?? 0}
          operatorPermission={memory?.operator_permission ?? 0}
          hasRoleWrite={hasRoleWrite}
          createTime={formatDate(memory?.create_time)}
        ></MemoryTable>
      </CardContent>
    </>
  );
}
