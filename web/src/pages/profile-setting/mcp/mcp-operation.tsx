import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { PenLine, Trash2, Upload } from 'lucide-react';
import { MouseEventHandler, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { UseEditMcpReturnType } from './use-edit-mcp';
import { useExportMcp } from './use-export-mcp';

export function McpOperation({
  mcpId,
  showEditModal,
}: { mcpId: string } & Pick<UseEditMcpReturnType, 'showEditModal'>) {
  const { t } = useTranslation();
  const { deleteMcpServer } = useDeleteMcpServer();
  const { handleExportMcpJson } = useExportMcp();

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteMcpServer([mcpId]);
  }, [deleteMcpServer, mcpId]);

  return (
    <div className="hidden gap-3  group-hover:flex text-text-secondary">
      <RAGFlowTooltip tooltip={t('mcp.export')}>
        <Upload
          className="size-3 cursor-pointer"
          onClick={handleExportMcpJson([mcpId])}
        />
      </RAGFlowTooltip>
      <RAGFlowTooltip tooltip={t('common.edit')}>
        <PenLine
          className="size-3 cursor-pointer"
          onClick={showEditModal(mcpId)}
        />
      </RAGFlowTooltip>
      <RAGFlowTooltip tooltip={t('common.delete')}>
        <ConfirmDeleteDialog onOk={handleDelete}>
          <Trash2 className="size-3 cursor-pointer" />
        </ConfirmDeleteDialog>
      </RAGFlowTooltip>
    </div>
  );
}
