import { FileIcon } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useDatasetEditButtonDisabled } from '@/ee/hooks/logic-hooks/use-permission';
import {
  hasDocumentManagementPermission,
  hasEditPermission,
  hasReadPermission,
} from '@/ee/utils/permission-util';
import { UseRowSelectionType } from '@/hooks/logic-hooks/use-row-selection';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { ColumnDef } from '@tanstack/table-core';
import { ArrowUpDown } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { MetadataType } from '../components/metedata/constant';
import { ShowManageMetadataModalProps } from '../components/metedata/interface';
import { DatasetActionCell } from './dataset-action-cell';
import { ParseCell, ParsingStatusCell } from './parsing-status-cell';
import { UseChangeDocumentParserShowType } from './use-change-document-parser';
import { UseRenameDocumentShowType } from './use-rename-document';

type UseDatasetTableColumnsType = UseChangeDocumentParserShowType &
  UseRenameDocumentShowType &
  Pick<UseRowSelectionType, 'setRowSelection'> & {
    showLog: (record: IDocumentInfo) => void;
    showManageMetadataModal: (config: ShowManageMetadataModalProps) => void;
  };

export function useDatasetTableColumns({
  showChangeParserModal,
  showRenameModal,
  showManageMetadataModal,
  showLog,
  setRowSelection,
}: UseDatasetTableColumnsType) {
  const datasetEditButtonDisabled = useDatasetEditButtonDisabled();
  const { t } = useTranslation('translation', {
    keyPrefix: 'knowledgeDetails',
  });
  // const { dataSourceInfo } = useDataSourceInfo();
  const { navigateToChunkParsedResult } = useNavigatePage();

  const columns: ColumnDef<IDocumentInfo>[] = useMemo(
    () => [
      {
        id: 'select',
        header: ({ table }) => {
          const canSelectAnyRow = table
            .getRowModel()
            .rows.some((row) => row.getCanSelect());
          return (
            <Checkbox
              checked={
                table.getIsAllPageRowsSelected() ||
                (table.getIsSomePageRowsSelected() && 'indeterminate')
              }
              onCheckedChange={(value) =>
                table.toggleAllPageRowsSelected(!!value)
              }
              aria-label="Select all"
              disabled={!canSelectAnyRow}
            />
          );
        },
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label="Select row"
            disabled={!row.getCanSelect()}
          />
        ),
        enableSorting: false,
        enableHiding: false,
      },
      {
        accessorKey: 'name',
        header: ({ column }) => {
          return (
            <div className="flex items-center gap-1">
              {t('name')}

              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() =>
                  column.toggleSorting(column.getIsSorted() === 'asc')
                }
              >
                <ArrowUpDown />
              </Button>
            </div>
          );
        },
        meta: { cellClassName: 'max-w-[20vw]' },
        cell: ({ row }) => {
          const name: string = row.getValue('name');
          const canReadDocument = hasReadPermission(
            row.original.operator_permission ?? 0,
          );

          return (
            <Tooltip>
              <TooltipTrigger asChild>
                <div
                  className={cn(
                    'flex items-center gap-2',
                    canReadDocument && 'cursor-pointer',
                  )}
                  aria-disabled={!canReadDocument}
                  onClick={
                    canReadDocument
                      ? navigateToChunkParsedResult(
                          row.original.id,
                          row.original.dataset_id,
                        )
                      : undefined
                  }
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
            <div className="flex items-center gap-1">
              {t('uploadDate')}

              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() =>
                  column.toggleSorting(column.getIsSorted() === 'asc')
                }
              >
                <ArrowUpDown />
              </Button>
            </div>
          );
        },
        cell: ({ row }) => (
          <time
            className="lowercase"
            dateTime={new Date(row.getValue('create_time')).toISOString()}
          >
            {formatDate(row.getValue('create_time'))}
          </time>
        ),
      },
      {
        accessorKey: 'chunk_count',
        header: t('chunkNumber'),
        cell: ({ row }) => (
          <div className="capitalize">{row.getValue('chunk_count')}</div>
        ),
      },
      {
        accessorKey: 'meta_fields',
        header: t('metadata.metadata'),
        cell: ({ row }) => {
          const metaFields =
            (row.getValue('meta_fields') as Record<string, any>) || {};
          const length = Object.entries(metaFields).filter(([, value]) => {
            if (value == null || value === '') return false;
            if (Array.isArray(value)) {
              return value.some((item) => item != null && item !== '');
            }
            return true;
          }).length;
          return (
            <Button
              variant="static"
              size="auto"
              disabled={
                !hasReadPermission(row.original.operator_permission ?? 0)
              }
              onClick={() => {
                showManageMetadataModal({
                  isEditField: false,
                  isCanAdd: hasEditPermission(
                    row.original.operator_permission ?? 0,
                  ),
                  isAddValue: hasEditPermission(
                    row.original.operator_permission ?? 0,
                  ),
                  type: MetadataType.UpdateSingle,
                  record: row.original,
                  title: (
                    <div className="flex flex-col gap-2 w-full">
                      <div className="text-base font-normal">
                        {t('metadata.editMetadata')}
                      </div>
                    </div>
                  ),
                  secondTitle: (
                    <div className="w-full flex gap-1 text-sm text-text-secondary">
                      <FileIcon name={row.original.name}></FileIcon>
                      <div className="truncate">{row.original.name}</div>
                    </div>
                  ),
                  isDeleteSingleValue: true,
                  readOnly: !hasEditPermission(
                    row.original.operator_permission ?? 0,
                  ),
                  documentIds: [row.original.id],
                });
              }}
            >
              {length + ' fields'}
            </Button>
          );
        },
      },
      {
        accessorKey: 'run',
        header: t('Parse'),
        cell: ({ row }) => {
          return (
            <ParseCell
              record={row.original}
              showChangeParserModal={showChangeParserModal}
              documentEditButtonDisabled={
                !hasEditPermission(row.original.operator_permission ?? 0)
              }
            />
          );
        },
      },
      {
        id: 'run-status',
        header: t('status'),
        cell: ({ row }) => {
          return <ParsingStatusCell record={row.original} showLog={showLog} />;
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
              setRowSelection={setRowSelection}
              datasetEditButtonDisabled={
                datasetEditButtonDisabled ||
                !hasEditPermission(record.operator_permission ?? 0)
              }
              documentEditButtonDisabled={
                !hasEditPermission(record.operator_permission ?? 0)
              }
              documentManageButtonDisabled={
                !hasDocumentManagementPermission(
                  record.operator_permission ?? 0,
                  !datasetEditButtonDisabled,
                )
              }
            />
          );
        },
      },
    ],
    [
      t,
      navigateToChunkParsedResult,
      showChangeParserModal,
      showRenameModal,
      showManageMetadataModal,
      showLog,
      setRowSelection,
      datasetEditButtonDisabled,
    ],
  );

  return columns;
}
