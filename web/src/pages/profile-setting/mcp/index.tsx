import { BulkOperateBar } from '@/components/bulk-operate-bar';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { pick } from 'lodash';
import { Import, Plus } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { EditMcpDialog } from './edit-mcp-dialog';
import { ImportMcpDialog } from './import-mcp-dialog';
import { McpCard } from './mcp-card';
import { useBulkOperateMCP } from './use-bulk-operate-mcp';
import { useEditMcp } from './use-edit-mcp';
import { useImportMcp } from './use-import-mcp';

export default function McpServer() {
  const { data, setPagination, searchString, handleInputChange, pagination } =
    useListMcpServer();
  const { editVisible, showEditModal, hideEditModal, handleOk, id, loading } =
    useEditMcp();
  const { list, selectedList, handleSelectChange } = useBulkOperateMCP();
  const { t } = useTranslation();
  const { importVisible, showImportModal, hideImportModal, onImportOk } =
    useImportMcp();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  return (
    <section className="p-4 w-full">
      <div className="text-text-primary text-2xl">MCP Servers</div>
      <section className="flex items-center justify-between pb-5">
        <div className="text-text-secondary">
          Customize the list of MCP servers
        </div>
        <div className="flex gap-5">
          <SearchInput
            className="w-40"
            value={searchString}
            onChange={handleInputChange}
          ></SearchInput>
          <Button variant={'secondary'} onClick={showImportModal}>
            <Import /> {t('mcp.import')}
          </Button>
          <Button onClick={showEditModal('')}>
            <Plus /> {t('mcp.addMCP')}
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
      <div className="mt-8 px-8">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={pagination.total || 0}
          onChange={handlePageChange}
        ></RAGFlowPagination>
      </div>
      {editVisible && (
        <EditMcpDialog
          hideModal={hideEditModal}
          onOk={handleOk}
          id={id}
          loading={loading}
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
