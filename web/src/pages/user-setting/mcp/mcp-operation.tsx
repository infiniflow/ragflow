import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { IMcpServer } from '@/interfaces/database/mcp';
import { PenLine, Trash2, Upload } from 'lucide-react';
import { useCallback } from 'react';
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

  const handleDelete = useCallback(() => {
    deleteMcpServer([mcp.id]);
  }, [deleteMcpServer, mcp.id]);

  return (
    <div className="hidden gap-1 group-hover:flex text-text-secondary">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 p-0"
            onClick={handleExportMcpJson([mcp.id])}
            aria-label={t('mcp.export')}
          >
            <Upload className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('mcp.export')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 p-0"
            onClick={showEditModal(mcp.id)}
            aria-label={t('common.edit')}
          >
            <PenLine className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('common.edit')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <ConfirmDeleteDialog
          onOk={handleDelete}
          title={t('common.delete') + ' ' + t('mcp.mcpServer')}
          content={{
            node: (
              <ConfirmDeleteDialogNode
                name={mcp.name}
              ></ConfirmDeleteDialogNode>
            ),
          }}
        >
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-8 w-8 p-0 hover:bg-state-error-5 hover:text-state-error"
              aria-label={t('common.delete')}
            >
              <Trash2 className="size-4" />
            </Button>
          </TooltipTrigger>
        </ConfirmDeleteDialog>
        <TooltipContent>{t('common.delete')}</TooltipContent>
      </Tooltip>
    </div>
  );
}
