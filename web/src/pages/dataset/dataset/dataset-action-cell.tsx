import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '@/components/ui/hover-card';
import { DocumentType } from '@/constants/knowledge';
import { useRemoveDocument } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { downloadDatasetDocument } from '@/services/file-manager-service';
import { formatFileSize } from '@/utils/common-util';
import { formatDate } from '@/utils/date';
import { downloadFileFromBlob } from '@/utils/file-util';
import { Download, Eye, PenLine, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { UseRenameDocumentShowType } from './use-rename-document';
import { isParserRunning } from './utils';

const Fields = ['name', 'size', 'type', 'create_time', 'update_time'];

const FunctionMap = {
  size: formatFileSize,
  create_time: formatDate,
  update_time: formatDate,
};

export function DatasetActionCell({
  record,
  showRenameModal,
}: { record: IDocumentInfo } & UseRenameDocumentShowType) {
  const { id, run, type } = record;
  const isRunning = isParserRunning(run);
  const isVirtualDocument = type === DocumentType.Virtual;

  const { removeDocument } = useRemoveDocument();

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

  const handleRemove = useCallback(() => {
    removeDocument(id);
  }, [id, removeDocument]);

  const handleRename = useCallback(() => {
    showRenameModal(record);
  }, [record, showRenameModal]);

  return (
    <div
      className="
      flex gap-2 items-center opacity-0
      transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
    >
      <Button
        size="icon-xs"
        variant="ghost"
        disabled={isRunning}
        onClick={handleRename}
      >
        <PenLine className="size-[1em]" />
      </Button>
      <HoverCard>
        <HoverCardTrigger>
          <Button size="icon-xs" variant="ghost" disabled={isRunning}>
            <Eye className="size-[1em]" />
          </Button>
        </HoverCardTrigger>
        <HoverCardContent className="w-[40vw] max-h-[40vh] overflow-auto">
          <ul className="space-y-2">
            {Object.entries(record)
              .filter(([key]) => Fields.some((x) => x === key))

              .map(([key, value], idx) => {
                return (
                  <li key={idx} className="flex gap-2">
                    {key}:
                    <div>
                      {key in FunctionMap
                        ? FunctionMap[key as keyof typeof FunctionMap](value)
                        : value}
                    </div>
                  </li>
                );
              })}
          </ul>
        </HoverCardContent>
      </HoverCard>

      {isVirtualDocument || (
        <Button
          size="icon-xs"
          variant="ghost"
          onClick={onDownloadDocument}
          disabled={isRunning}
        >
          <Download className="size-[1em]" />
        </Button>
      )}
      <ConfirmDeleteDialog onOk={handleRemove}>
        <Button
          data-testid="document-delete"
          size="icon-xs"
          variant="ghost"
          disabled={isRunning}
        >
          <Trash2 className="size-[1em]" />
        </Button>
      </ConfirmDeleteDialog>
    </div>
  );
}
