import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { MoreButton } from '@/components/more-button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { Switch } from '@/components/ui/switch';
import { DocumentType } from '@/constants/knowledge';
import {
  DocumentPrivilegeAction,
  useDocumentActionCellOverrides,
} from '@/ee/pages/dataset/dataset/dataset-action-cell-seams';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import {
  useRemoveDocument,
  useSetDocumentStatus,
} from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { downloadDatasetDocument } from '@/services/file-manager-service';
import { formatFileSize } from '@/utils/common-util';
import { formatDate } from '@/utils/date';
import { downloadFileFromBlob } from '@/utils/file-util';
import { Download, Eye, PenLine, Trash2 } from 'lucide-react';
import { omit } from 'lodash';
import { MouseEvent, useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UseRenameDocumentShowType } from './use-rename-document';
import { isDocumentProcessing } from './utils';

const Fields = ['name', 'size', 'type', 'create_time', 'update_time'];

const FunctionMap = {
  size: formatFileSize,
  create_time: formatDate,
  update_time: formatDate,
};

type DatasetActionCellProps = {
  record: IDocumentInfo;
  datasetEditButtonDisabled?: boolean;
} & UseRenameDocumentShowType &
  Pick<UseRowSelectionType, 'setRowSelection'>;

export function DatasetActionCell({
  record,
  showRenameModal,
  setRowSelection,
  datasetEditButtonDisabled,
}: DatasetActionCellProps) {
  const { t } = useTranslation();
  const { id, type } = record;
  const [menuOpen, setMenuOpen] = useState(false);
  const isRunning = isDocumentProcessing(record);
  const isVirtualDocument = type === DocumentType.Virtual;
  const actionDisabled = isRunning || Boolean(datasetEditButtonDisabled);
  const enabled = record.status === '1';
  const statusLabel = enabled
    ? t('knowledgeDetails.enabledStatus')
    : t('knowledgeDetails.disabledStatus');
  const { actionClassName } = useDocumentActionCellOverrides();

  const { removeDocument } = useRemoveDocument();
  const { setDocumentStatus, loading: statusLoading } = useSetDocumentStatus();

  const handleStatusChange = useCallback(
    (status: boolean) => {
      void setDocumentStatus({
        status,
        documentId: id,
        datasetId: record.dataset_id,
      });
    },
    [id, record.dataset_id, setDocumentStatus],
  );

  const onDownloadDocument = useCallback(async () => {
    try {
      const ext = record.name.split('.').pop()?.toLowerCase() || 'bin';
      const response = await downloadDatasetDocument({
        datasetId: record.dataset_id,
        docId: id,
        ext,
      });
      const blob = new Blob([response.data], {
        type: response.data.type,
      });
      downloadFileFromBlob(blob, record.name);
    } catch (error) {
      console.error('Error downloading document:', error);
    }
  }, [id, record.dataset_id, record.name]);

  const handleRemove = useCallback(async () => {
    const code = await removeDocument(id);
    if (code === 0) {
      setRowSelection((prev) => omit(prev, [id]));
    }
  }, [id, removeDocument, setRowSelection]);

  const handleRename = useCallback(() => {
    showRenameModal(record);
  }, [record, showRenameModal]);

  const handleKeepMenuOpen = useCallback((event: Event) => {
    event.preventDefault();
  }, []);

  const handleMenuItemClick = useCallback(
    (event: MouseEvent<HTMLDivElement>) => {
      event.stopPropagation();
    },
    [],
  );

  return (
    <div
      className={cn(
        'flex gap-2 items-center opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100',
        menuOpen && 'opacity-100',
        actionClassName,
      )}
    >
      <DocumentPrivilegeAction record={record} disabled={actionDisabled} />
      <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
        <DropdownMenuTrigger asChild>
          <MoreButton aria-label={t('knowledgeDetails.action')} />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-40">
          <DropdownMenuLabel className="flex items-center justify-between gap-4 font-normal">
            <span className="flex items-center gap-2">
              <span
                className={cn(
                  'size-1.5 rounded-full',
                  enabled ? 'bg-accent-primary' : 'bg-text-disabled',
                )}
              />
              {statusLabel}
            </span>
            <Switch
              checked={enabled}
              onCheckedChange={handleStatusChange}
              disabled={statusLoading}
              aria-label={statusLabel}
            />
          </DropdownMenuLabel>
          <DropdownMenuItem
            justifyBetween={false}
            disabled={actionDisabled}
            onClick={handleRename}
          >
            <PenLine />
            {t('common.rename')}
          </DropdownMenuItem>
          <HoverCard>
            <HoverCardTrigger asChild>
              <DropdownMenuItem
                justifyBetween={false}
                disabled={isRunning}
                onSelect={handleKeepMenuOpen}
              >
                <Eye />
                {t('knowledgeDetails.info')}
              </DropdownMenuItem>
            </HoverCardTrigger>
            <HoverCardContent
              side="left"
              align="start"
              className="w-[40vw] max-h-[40vh] overflow-auto"
            >
              <ul className="space-y-2">
                {Object.entries(record)
                  .filter(([key]) => Fields.some((x) => x === key))
                  .map(([key, value], idx) => {
                    return (
                      <li key={idx} className="flex gap-2">
                        {key}:
                        <div>
                          {key in FunctionMap
                            ? FunctionMap[key as keyof typeof FunctionMap](
                                value,
                              )
                            : value}
                        </div>
                      </li>
                    );
                  })}
              </ul>
            </HoverCardContent>
          </HoverCard>
          {isVirtualDocument || (
            <DropdownMenuItem
              justifyBetween={false}
              onClick={onDownloadDocument}
              disabled={isRunning}
            >
              <Download />
              {t('common.download')}
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <ConfirmDeleteDialog onOk={handleRemove}>
            <DropdownMenuItem
              data-testid="document-delete"
              justifyBetween={false}
              className="text-state-error"
              disabled={actionDisabled}
              onSelect={handleKeepMenuOpen}
              onClick={handleMenuItemClick}
            >
              <Trash2 />
              {t('common.delete')}
            </DropdownMenuItem>
          </ConfirmDeleteDialog>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
