/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { CardContainer } from '@/components/card-container';
import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { PrivilegeManagementDialog } from '@/ee/components/privilege-management/privilege-management-dialog';
import {
  McpEmptyStateGuard,
  McpSelectionBarGuard,
  McpToolbarGuard,
} from '@/ee/pages/user-setting/mcp/mcp-list-guard';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { SearchInput } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useGoToPreviousPageOnEmpty } from '@/hooks/logic-hooks';
import { useClearSelectionOnPageChange } from '@/hooks/logic-hooks/use-clear-selection-on-page-change';
import { useExitMcpSelectionWithoutOwner } from '@/ee/pages/user-setting/mcp/use-exit-mcp-selection-without-owner';
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
import { useShowPrivilegeDialog } from '@/ee/pages/user-setting/mcp/use-show-privilege-dialog';

export default function McpServer() {
  const {
    editVisible,
    showEditModal,
    hideEditModal,
    handleOk,
    id,
    loading,
    operatorPermission,
  } = useEditMcp();
  const {
    data,
    setPagination,
    searchString,
    setSearchString,
    handleInputChange,
    pagination,
    loading: listLoading,
  } = useListMcpServer();
  useGoToPreviousPageOnEmpty(data.mcp_servers?.length, listLoading, {
    deletionKey: ListDeletionKey.McpServerList,
    searchString,
    setSearchString,
  });
  const {
    selectedList,
    handleSelectChange,
    handleDelete,
    handleExportMcp,
    handleSelectAll,
    selectAllChecked,
    resetSelection,
  } = useBulkOperateMCP(data.mcp_servers);
  useClearSelectionOnPageChange(pagination, resetSelection);
  const { t } = useTranslation();
  const {
    privilegeModal,
    hidePrivilegeModal,
    handShowPrivilegeModal,
    recordWithSourceType,
  } = useShowPrivilegeDialog();
  const {
    importVisible,
    showImportModal,
    hideImportModal,
    onImportOk,
    loading: importLoading,
  } = useImportMcp();

  const [isSelectionMode, setSelectionMode] = useState(false);

  const exitSelectionMode = useCallback(() => {
    setSelectionMode(false);
  }, []);

  useExitMcpSelectionWithoutOwner(
    data.mcp_servers,
    isSelectionMode,
    exitSelectionMode,
  );

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );

  const switchSelectionMode = useCallback(() => {
    setSelectionMode((prev) => !prev);
    resetSelection();
  }, [resetSelection]);

  return (
    <ProfileSettingWrapperCard
      header={
        <header className="flex flex-row gap-1.5 justify-between items-end">
          <div>
            <h2 className="text-text-primary text-2xl font-medium">
              {t('mcp.mcpServers')}
            </h2>

            <p className="mt-1 text-text-secondary text-sm">
              {t('mcp.customizeTheListOfMcpServers')}
            </p>
          </div>

          <div className="flex gap-4" role="toolbar">
            <SearchInput
              className="w-40"
              value={searchString}
              onChange={handleInputChange}
              placeholder={t('common.search')}
            />
            <McpToolbarGuard
              mcpList={data.mcp_servers}
              bulkManageButton={
                <Button variant="outline" onClick={switchSelectionMode}>
                  {isSelectionMode ? (
                    <ListChecks className="size-3.5" />
                  ) : (
                    <LayoutList className="size-3.5" />
                  )}
                  {t(
                    `mcp.${isSelectionMode ? 'exitBulkManage' : 'bulkManage'}`,
                  )}
                </Button>
              }
            >
              <Button variant="outline" onClick={showImportModal}>
                <Download className="size-3.5" />
                {t('mcp.import')}
              </Button>
              <Button onClick={showEditModal('')}>
                <Plus /> {t('mcp.addMCP')}
              </Button>
            </McpToolbarGuard>
          </div>
        </header>
      }
    >
      <div className="h-full p-5 overflow-x-hidden overflow-y-auto">
        {data.mcp_servers?.length ? (
          <>
            {isSelectionMode && (
              <McpSelectionBarGuard mcpList={data.mcp_servers}>
                <section className="pb-5 flex items-center">
                  <Checkbox
                    id="all"
                    checked={selectAllChecked}
                    onCheckedChange={handleSelectAll}
                  />
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
              </McpSelectionBarGuard>
            )}
            <CardContainer>
              {data.mcp_servers.map((item) => (
                <McpCard
                  key={item.id}
                  data={item}
                  selectedList={selectedList}
                  handleSelectChange={handleSelectChange}
                  showEditModal={showEditModal}
                  showPrivilegeModal={handShowPrivilegeModal(item)}
                  isSelectionMode={isSelectionMode}
                />
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
        ) : (
          <McpEmptyStateGuard
            mcpList={data.mcp_servers}
            addButton={
              <Button variant="outline">
                <Plus />
                {t('empty.addNow')}
              </Button>
            }
            onClickAdd={showEditModal('')}
          >
            <div className="text-text-secondary text-sm">
              {t('empty.noMCP')}
            </div>
          </McpEmptyStateGuard>
        )}
      </div>

      {editVisible && (
        <EditMcpDialog
          hideModal={hideEditModal}
          onOk={handleOk}
          id={id}
          loading={loading}
          operatorPermission={operatorPermission}
        ></EditMcpDialog>
      )}
      {importVisible && (
        <ImportMcpDialog
          hideModal={hideImportModal}
          onOk={onImportOk}
          loading={importLoading}
        ></ImportMcpDialog>
      )}
      {privilegeModal && (
        <PrivilegeManagementDialog
          hideModal={hidePrivilegeModal}
          initialValues={recordWithSourceType}
        />
      )}
    </ProfileSettingWrapperCard>
  );
}
