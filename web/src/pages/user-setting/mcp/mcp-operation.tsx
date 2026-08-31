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

import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { IMcpServer } from '@/interfaces/database/mcp';
import { PenLine, Trash2, Upload } from 'lucide-react';
import { MouseEventHandler, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { UseEditMcpReturnType } from './use-edit-mcp';
import { useExportMcp } from './use-export-mcp';

export function McpOperation({
  mcp,
  showEditModal,
}: { mcp: IMcpServer } & Pick<UseEditMcpReturnType, 'showEditModal'>) {
  const { t } = useTranslation();
  const { deleteMcpServer } = useDeleteMcpServer();
  const { handleExportMcpJson } = useExportMcp(mcp);

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteMcpServer([mcp.id]);
  }, [deleteMcpServer, mcp.id]);

  return (
    <div className="hidden gap-1  group-hover:flex text-text-secondary">
      {/* <RAGFlowTooltip tooltip={t('mcp.export')}> */}
      <Upload
        className="size-5 cursor-pointer p-1 rounded-sm hover:text-text-primary hover:bg-bg-card"
        onClick={handleExportMcpJson([mcp.id])}
      />
      {/* </RAGFlowTooltip>
      <RAGFlowTooltip tooltip={t('common.edit')}> */}
      <PenLine
        className="size-5 cursor-pointer p-1 rounded-sm hover:text-text-primary hover:bg-bg-card"
        onClick={showEditModal(mcp.id)}
      />
      {/* </RAGFlowTooltip>
      <RAGFlowTooltip tooltip={t('common.delete')}> */}
      <ConfirmDeleteDialog
        onOk={handleDelete}
        title={t('common.delete') + ' ' + t('mcp.mcpServer')}
        content={{
          node: (
            <ConfirmDeleteDialogNode name={mcp.name}></ConfirmDeleteDialogNode>
          ),
        }}
      >
        <Trash2 className="size-5 cursor-pointer p-1 rounded-sm hover:text-state-error hover:bg-state-error-5" />
      </ConfirmDeleteDialog>
      {/* </RAGFlowTooltip> */}
    </div>
  );
}
