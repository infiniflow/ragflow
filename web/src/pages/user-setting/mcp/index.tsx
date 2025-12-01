import { CardContainer } from '@/components/card-container';
import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useListMcpServer } from '@/hooks/use-mcp-request';
import { pick } from 'lodash';
import {
  Download,
  LayoutList,
  ListChecks,
  Plus,
  Trash2,
  Upload,
} from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProfileSettingWrapperCard } from '../components/user-setting-header';
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
  const {
    selectedList,
    handleSelectChange,
    handleDelete,
    handleExportMcp,
    handleSelectAll,
  } = useBulkOperateMCP(data.mcp_servers);
  const { t } = useTranslation();
  const { importVisible, showImportModal, hideImportModal, onImportOk } =
    useImportMcp();

  const [isSelectionMode, setSelectionMode] = useState(false);

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const switchSelectionMode = useCallback(() => {
    setSelectionMode((prev) => !prev);
  }, []);

  return (
    <ProfileSettingWrapperCard
      header={
        <>
          <div className="text-text-primary text-2xl font-semibold">
            {t('mcp.mcpServers')}
          </div>
          <section className="flex items-center justify-between">
            <div className="text-text-secondary">
              {t('mcp.customizeTheListOfMcpServers')}
            </div>
            <div className="flex gap-5">
              <SearchInput
                className="w-40"
                value={searchString}
                onChange={handleInputChange}
                placeholder={t('common.search')}
              ></SearchInput>
              <Button variant={'secondary'} onClick={switchSelectionMode}>
                {isSelectionMode ? (
                  <ListChecks className="size-3.5" />
                ) : (
                  <LayoutList className="size-3.5" />
                )}
                {t(`mcp.${isSelectionMode ? 'exitBulkManage' : 'bulkManage'}`)}
              </Button>
              <Button variant={'secondary'} onClick={showImportModal}>
                <Download className="size-3.5" />
                {t('mcp.import')}
              </Button>
              <Button onClick={showEditModal('')}>
                <Plus className="size-3.5 font-medium" /> {t('mcp.addMCP')}
              </Button>
            </div>
          </section>
        </>
      }
    >
      {!data.mcp_servers?.length && (
        <div
          className="flex items-center justify-between border border-dashed border-border-button rounded-md p-10 cursor-pointer w-[590px]"
          onClick={showEditModal('')}
        >
          <div className="text-text-secondary text-sm">{t('empty.noMCP')}</div>
          <Button variant={'ghost'} className="border border-border-button">
            <Plus className="size-3.5 font-medium" /> {t('empty.addNow')}
          </Button>
        </div>
      )}
      {!!data.mcp_servers?.length && (
        <>
          {isSelectionMode && (
            <section className="pb-5 flex items-center">
              <Checkbox id="all" onCheckedChange={handleSelectAll} />
              <Label
                className="pl-2 text-text-primary cursor-pointer"
                htmlFor="all"
              >
                {t('common.selectAll')}
              </Label>
              <span className="text-text-secondary pr-10 pl-5">
                {t('mcp.selected')} {selectedList.length}
              </span>
              <div className="flex gap-10 items-center">
                <Button variant={'secondary'} onClick={handleExportMcp}>
                  <Upload className="size-3.5"></Upload>
                  {t('mcp.export')}
                </Button>
                <ConfirmDeleteDialog
                  onOk={handleDelete}
                  title={t('common.delete') + ' ' + t('mcp.mcpServers')}
                  content={{
                    title: t('common.deleteThem'),
                    node: (
                      <ConfirmDeleteDialogNode
                        name={`${t('mcp.selected')} ${selectedList.length} ${t('mcp.mcpServers')}`}
                      ></ConfirmDeleteDialogNode>
                    ),
                  }}
                >
                  <Button variant={'danger'}>
                    <Trash2 className="size-3.5 cursor-pointer" />
                    {t('common.delete')}
                  </Button>
                </ConfirmDeleteDialog>
              </div>
            </section>
          )}
          <CardContainer>
            {data.mcp_servers.map((item) => (
              <McpCard
                key={item.id}
                data={item}
                selectedList={selectedList}
                handleSelectChange={handleSelectChange}
                showEditModal={showEditModal}
                isSelectionMode={isSelectionMode}
              ></McpCard>
            ))}
          </CardContainer>
          <div className="mt-8">
            <RAGFlowPagination
              {...pick(pagination, 'current', 'pageSize')}
              total={pagination.total || 0}
              onChange={handlePageChange}
            ></RAGFlowPagination>
          </div>
        </>
      )}
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
      <Spotlight />
    </ProfileSettingWrapperCard>
  );
}
