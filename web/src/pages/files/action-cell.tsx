import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import NewDocumentLink from '@/components/new-document-link';
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
import {
  getExtension,
  isSupportedPreviewDocumentType,
} from '@/utils/document-util';
import { CellContext } from '@tanstack/react-table';
import { EllipsisVertical, Eye, Link2, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  UseHandleConnectToKnowledgeReturnType,
  UseRenameCurrentFileReturnType,
} from './hooks';
import { UseMoveDocumentShowType } from './use-move-file';
import { isFolderType } from './util';

type IProps = Pick<CellContext<IFile, unknown>, 'row'> &
  Pick<UseHandleConnectToKnowledgeReturnType, 'showConnectToKnowledgeModal'> &
  Pick<UseRenameCurrentFileReturnType, 'showFileRenameModal'> &
  UseMoveDocumentShowType;

export function ActionCell({
  row,
  showConnectToKnowledgeModal,
  showFileRenameModal,
  showMoveFileModal,
}: IProps) {
  const { t } = useTranslation();
  const record = row.original;
  const documentId = record.id;
  const { downloadFile } = useDownloadFile();
  const isFolder = isFolderType(record.type);
  const extension = getExtension(record.name);

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

  const handleShowMoveFileModal = useCallback(() => {
    showMoveFileModal([record.id]);
  }, [record, showMoveFileModal]);

  return (
    <section className="flex gap-4 items-center">
      <Button
        variant="ghost"
        size={'icon'}
        onClick={handleShowConnectToKnowledgeModal}
      >
        <Link2 />
      </Button>
      <ConfirmDeleteDialog>
        <Button variant="ghost" size={'icon'}>
          <Trash2 className="text-text-delete-red" />
        </Button>
      </ConfirmDeleteDialog>
      {isSupportedPreviewDocumentType(extension) && (
        <NewDocumentLink
          documentId={documentId}
          documentName={record.name}
          color="black"
        >
          <Button variant={'ghost'} size={'icon'}>
            <Eye />
          </Button>
        </NewDocumentLink>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size={'icon'}>
            <EllipsisVertical />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={handleShowMoveFileModal}>
            {t('common.move')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={handleShowFileRenameModal}>
            {t('common.rename')}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          {isFolder || (
            <DropdownMenuItem onClick={onDownloadDocument}>
              {t('common.download')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </section>
  );
}
