import SvgIcon from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Switch } from '@/components/ui/switch';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IDocumentInfo } from '@/interfaces/database/document';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { getExtension } from '@/utils/document-util';
import { ColumnDef } from '@tanstack/table-core';
import { ArrowUpDown, MoreHorizontal, Pencil, Wrench } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useChangeDocumentParser } from './hooks';

type UseDatasetTableColumnsType = Pick<
  ReturnType<typeof useChangeDocumentParser>,
  'showChangeParserModal'
> & { setCurrentRecord: (record: IDocumentInfo) => void };

export function useDatasetTableColumns({
  showChangeParserModal,
  setCurrentRecord,
}: UseDatasetTableColumnsType) {
  const { t } = useTranslation('translation', {
    keyPrefix: 'knowledgeDetails',
  });

  // const onShowRenameModal = (record: IDocumentInfo) => {
  //   setCurrentRecord(record);
  //   showRenameModal();
  // };
  const onShowChangeParserModal = useCallback(
    (record: IDocumentInfo) => () => {
      setCurrentRecord(record);
      showChangeParserModal();
    },
    [setCurrentRecord, showChangeParserModal],
  );

  // const onShowSetMetaModal = useCallback(() => {
  //   setRecord();
  //   showSetMetaModal();
  // }, [setRecord, showSetMetaModal]);

  const { navigateToChunkParsedResult } = useNavigatePage();

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
            variant="ghost"
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
                <SvgIcon
                  name={`file-icon/${getExtension(name)}`}
                  width={24}
                ></SvgIcon>
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
            variant="ghost"
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
      accessorKey: 'parser_id',
      header: t('chunkMethod'),
      cell: ({ row }) => (
        <div className="capitalize">{row.getValue('parser_id')}</div>
      ),
    },
    {
      accessorKey: 'run',
      header: t('parsingStatus'),
      cell: ({ row }) => (
        <Button variant="destructive" size={'sm'}>
          {row.getValue('run')}
        </Button>
      ),
    },
    {
      id: 'actions',
      header: t('action'),
      enableHiding: false,
      cell: ({ row }) => {
        const record = row.original;

        return (
          <section className="flex gap-4 items-center">
            <Switch id="airplane-mode" />
            <Button
              variant="icon"
              size={'icon'}
              onClick={onShowChangeParserModal(record)}
            >
              <Wrench />
            </Button>
            <Button variant="icon" size={'icon'}>
              <Pencil />
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="icon" size={'icon'}>
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuLabel>Actions</DropdownMenuLabel>
                <DropdownMenuItem
                  onClick={() => navigator.clipboard.writeText(record.id)}
                >
                  Copy payment ID
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem>View customer</DropdownMenuItem>
                <DropdownMenuItem>View payment details</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </section>
        );
      },
    },
  ];

  return columns;
}
