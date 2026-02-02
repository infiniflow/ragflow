import { FileIcon } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Switch } from '@/components/ui/switch';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useSetDocumentStatus } from '@/hooks/use-document-request';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { useDataSourceInfo } from '@/pages/user-setting/data-source/constant';
import { formatDate } from '@/utils/date';
import { ColumnDef } from '@tanstack/table-core';
import { ArrowUpDown, MonitorUp } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MetadataType } from '../components/metedata/constant';
import { ShowManageMetadataModalProps } from '../components/metedata/interface';
import { DatasetActionCell } from './dataset-action-cell';
import { ParsingStatusCell } from './parsing-status-cell';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { UseRenameDocumentShowType } from './use-rename-document';

type UseDatasetTableColumnsType = UseChangeDocumentParserShowType &
  UseRenameDocumentShowType & {
    showLog: (record: IDocumentInfo) => void;
    showManageMetadataModal: (config: ShowManageMetadataModalProps) => void;
  };

export function useDatasetTableColumns({
  showChangeParserModal,
  showRenameModal,
  showManageMetadataModal,
  showLog,
}: UseDatasetTableColumnsType) {
  const { t } = useTranslation('translation', {
    keyPrefix: 'knowledgeDetails',
  });
  const { dataSourceInfo } = useDataSourceInfo();
  const { navigateToChunkParsedResult } = useNavigatePage();
  const { setDocumentStatus } = useSetDocumentStatus();

  const columns: ColumnDef<IDocumentInfo>[] = [
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={
            table.getIsAllPageRowsSelected() ||
            (table.getIsSomePageRowsSelected() && 'indeterminate')
          }
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label="Select all"
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label="Select row"
        />
      ),
      enableSorting: false,
      enableHiding: false,
    },
    {
      accessorKey: 'name',
      header: ({ column }) => {
        return (
          <Button
            variant="transparent"
            className="border-none"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('name')}
            <ArrowUpDown />
          </Button>
        );
      },
      meta: { cellClassName: 'max-w-[20vw]' },
      cell: ({ row }) => {
        const name: string = row.getValue('name');

        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div
                className="flex gap-2 cursor-pointer"
                onClick={navigateToChunkParsedResult(
                  row.original.id,
                  row.original.kb_id,
                )}
              >
                <FileIcon name={name}></FileIcon>
                <span className={cn('truncate')}>{name}</span>
              </div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{name}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'create_time',
      header: ({ column }) => {
        return (
          <Button
            variant="transparent"
            className="border-none"
            onClick={() => column.toggleSorting(column.getIsSorted() === 'asc')}
          >
            {t('uploadDate')}
            <ArrowUpDown />
          </Button>
        );
      },
      cell: ({ row }) => (
        <div className="lowercase">
          {formatDate(row.getValue('create_time'))}
        </div>
      ),
    },
    {
      accessorKey: 'source_from',
      header: t('source'),
      cell: ({ row }) => (
        <div className="text-text-primary">
          {row.original.source_type === 'local' ||
          row.original.source_type === '' ? (
            <div className="bg-accent-primary-5 w-6 h-6 rounded-full flex items-center justify-center">
              <MonitorUp className="text-accent-primary" size={16} />
            </div>
          ) : (
            <div className="w-6 h-6 flex items-center justify-center">
              {
                dataSourceInfo[
                  row.original.source_type as keyof typeof dataSourceInfo
                ]?.icon
              }
            </div>
          )}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('enabled'),
      cell: ({ row }) => {
        const id = row.original.id;
        return (
          <Switch
            checked={row.getValue('status') === '1'}
            onCheckedChange={(e) => {
              setDocumentStatus({ status: e, documentId: id });
            }}
          />
        );
      },
    },
    {
      accessorKey: 'chunk_num',
      header: t('chunkNumber'),
      cell: ({ row }) => (
        <div className="capitalize">{row.getValue('chunk_num')}</div>
      ),
    },
    {
      accessorKey: 'meta_fields',
      header: t('metadata.metadata'),
      cell: ({ row }) => {
        const length = Object.keys(row.getValue('meta_fields') || {}).length;
        return (
          <div
            className="capitalize cursor-pointer"
            onClick={() => {
              showManageMetadataModal({
                // metadata: util.JSONToMetaDataTableData(
                //   row.original.meta_fields || {},
                // ),
                isEditField: false,
                isCanAdd: true,
                isAddValue: true,
                type: MetadataType.UpdateSingle,
                record: row.original,
                title: (
                  <div className="flex flex-col gap-2 w-full">
                    <div className="text-base font-normal">
                      {t('metadata.editMetadata')}
                    </div>
                    {/* <div className="text-sm text-text-secondary w-full truncate">
                      {t('metadata.editMetadataForDataset')}
                      {row.original.name}
                    </div> */}
                  </div>
                ),
                secondTitle: (
                  <div className="w-full flex gap-1 text-sm text-text-secondary">
                    <FileIcon name={row.original.name}></FileIcon>
                    <div className="truncate">{row.original.name}</div>
                  </div>
                ),
                isDeleteSingleValue: true,
                documentIds: [row.original.id],
              });
            }}
          >
            {length + ' fields'}
          </div>
        );
      },
    },
    {
      accessorKey: 'run',
      header: t('Parse'),
      // meta: { cellClassName: 'min-w-[20vw]' },
      cell: ({ row }) => {
        return (
          <ParsingStatusCell
            record={row.original}
            showChangeParserModal={showChangeParserModal}
            showLog={showLog}
          ></ParsingStatusCell>
        );
      },
    },
    {
      id: 'actions',
      header: t('action'),
      enableHiding: false,
      cell: ({ row }) => {
        const record = row.original;

        return (
          <DatasetActionCell
            record={record}
            showRenameModal={showRenameModal}
          ></DatasetActionCell>
        );
      },
    },
  ];

  return columns;
}
