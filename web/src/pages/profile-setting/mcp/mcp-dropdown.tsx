import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDeleteMcpServer } from '@/hooks/use-mcp-request';
import { PenLine, Trash2, Upload } from 'lucide-react';
import { MouseEventHandler, PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { UseEditMcpReturnType } from './use-edit-mcp';
import { useExportMcp } from './use-export-mcp';

export function McpDropdown({
  children,
  mcpId,
  showEditModal,
}: PropsWithChildren & { mcpId: string } & Pick<
    UseEditMcpReturnType,
    'showEditModal'
  >) {
  const { t } = useTranslation();
  const { deleteMcpServer } = useDeleteMcpServer();
  const { handleExportMcpJson } = useExportMcp();

  const handleDelete: MouseEventHandler<HTMLDivElement> = useCallback(() => {
    deleteMcpServer([mcpId]);
  }, [deleteMcpServer, mcpId]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent>
        <DropdownMenuItem onClick={showEditModal(mcpId)}>
          {t('common.edit')} <PenLine />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem onClick={handleExportMcpJson([mcpId])}>
          {t('mcp.export')} <Upload />
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <ConfirmDeleteDialog onOk={handleDelete}>
          <DropdownMenuItem
            className="text-state-error"
            onSelect={(e) => {
              e.preventDefault();
            }}
            onClick={(e) => {
              e.stopPropagation();
            }}
          >
            {t('common.delete')} <Trash2 />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
