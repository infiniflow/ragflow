import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { Import, Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { EditMcpDialog } from './edit-mcp-dialog';
import { ImportMcpDialog } from './import-mcp-dialog';
import { McpCard } from './mcp-card';
import { useBulkOperateMCP } from './use-bulk-operate-mcp';
import { useEditMcp } from './use-edit-mcp';
import { useImportMcp } from './use-import-mcp';

export default function McpServer() {
  const { data } = useListMcpServer();
  const { editVisible, showEditModal, hideEditModal, handleOk, id } =
    useEditMcp();
  const { list, selectedList, handleSelectChange } = useBulkOperateMCP();
  const { t } = useTranslation();
  const { importVisible, showImportModal, hideImportModal, onImportOk } =
    useImportMcp();

  return (
    <section className="p-4">
      <div className="text-text-title text-2xl">MCP Servers</div>
      <section className="flex items-center justify-between pb-5">
        <div className="text-text-sub-title">
          Customize the list of MCP servers
        </div>
        <div className="flex gap-5">
          <SearchInput className="w-40"></SearchInput>
          <Button variant={'secondary'} onClick={showImportModal}>
            <Import /> {t('mcp.import')}
          </Button>
          <Button onClick={showEditModal('')}>
            <Plus /> {t('mcp.addMcp')}
          </Button>
        </div>
      </section>
      {selectedList.length > 0 && (
        <BulkOperateBar
          list={list}
          count={selectedList.length}
          className="mb-2.5"
        ></BulkOperateBar>
      )}
      <section className="flex gap-5 flex-wrap">
        {data.mcp_servers.map((item) => (
          <McpCard
            key={item.id}
            data={item}
            selectedList={selectedList}
            handleSelectChange={handleSelectChange}
            showEditModal={showEditModal}
          ></McpCard>
        ))}
      </section>
      {editVisible && (
        <EditMcpDialog
          hideModal={hideEditModal}
          onOk={handleOk}
          id={id}
        ></EditMcpDialog>
      )}
      {importVisible && (
        <ImportMcpDialog
          hideModal={hideImportModal}
          onOk={onImportOk}
        ></ImportMcpDialog>
      )}
    </section>
  );
}
