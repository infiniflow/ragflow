import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useDownloadFile } from '@/hooks/file-manager-hooks';
import { IFile } from '@/interfaces/database/file-manager';
import { CellContext } from '@tanstack/react-table';
import { EllipsisVertical, Link2, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  UseHandleConnectToKnowledgeReturnType,
  UseRenameCurrentFileReturnType,
} from './hooks';

type IProps = Pick<CellContext<IFile, unknown>, 'row'> &
  Pick<UseHandleConnectToKnowledgeReturnType, 'showConnectToKnowledgeModal'> &
  Pick<UseRenameCurrentFileReturnType, 'showFileRenameModal'>;

export function ActionCell({
  row,
  showConnectToKnowledgeModal,
  showFileRenameModal,
}: IProps) {
  const { t } = useTranslation();
  const record = row.original;
  const documentId = record.id;
  const { downloadFile } = useDownloadFile();

  const handleShowConnectToKnowledgeModal = useCallback(() => {
    showConnectToKnowledgeModal(record);
  }, [record, showConnectToKnowledgeModal]);

  const onDownloadDocument = useCallback(() => {
    downloadFile({
      id: documentId,
      filename: record.name,
    });
  }, [documentId, downloadFile, record.name]);

  const handleShowFileRenameModal = useCallback(() => {
    showFileRenameModal(record);
  }, [record, showFileRenameModal]);

  return (
    <section className="flex gap-4 items-center">
      <Button
        variant="secondary"
        size={'icon'}
        onClick={handleShowConnectToKnowledgeModal}
      >
        <Link2 />
      </Button>
      <ConfirmDeleteDialog>
        <Button variant="secondary" size={'icon'}>
          <Trash2 />
        </Button>
      </ConfirmDeleteDialog>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="secondary" size={'icon'}>
            <EllipsisVertical />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            onClick={() => navigator.clipboard.writeText(record.id)}
          >
            {t('common.move')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={handleShowFileRenameModal}>
            {t('common.rename')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onDownloadDocument}>
            {t('common.download')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </section>
  );
}
