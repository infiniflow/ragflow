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
  const { handleExportMcpJson } = useExportMcp();

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
