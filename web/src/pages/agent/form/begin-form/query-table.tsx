'use client';

import {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
  VisibilityState,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { Pencil, Trash2 } from 'lucide-react';
import * as React from 'react';

import { DataTable } from '@/components/data-table';
import { Button } from '@/components/ui/button';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { useTranslation } from 'react-i18next';
import { BeginQuery } from '../../interface';

interface IProps {
  data: BeginQuery[];
  deleteRecord(index: number): void;
  showModal(index: number, record: BeginQuery): void;
}

export function QueryTable({ data = [], deleteRecord, showModal }: IProps) {
  const { t } = useTranslation();

  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});

  const columns: ColumnDef<BeginQuery>[] = [
    {
      accessorKey: 'key',
      header: t('flow.key'),
      meta: { cellClassName: 'max-w-30' },
      cell: ({ row }) => {
        const key: string = row.getValue('key');
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="truncate ">{key}</div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{key}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'name',
      header: t('flow.name'),
      meta: { cellClassName: 'max-w-30' },
      cell: ({ row }) => {
        const name: string = row.getValue('name');
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="truncate">{name}</div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{name}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'type',
      header: t('flow.type'),
      cell: ({ row }) => (
        <div>
          {t(`flow.${(row.getValue('type')?.toString() || '').toLowerCase()}`)}
        </div>
      ),
    },
    {
      accessorKey: 'optional',
      header: t('flow.optional'),
      cell: ({ row }) => <div>{row.getValue('optional') ? 'Yes' : 'No'}</div>,
    },
    {
      id: 'actions',
      enableHiding: false,
      header: t('common.action'),
      cell: ({ row }) => {
        const record = row.original;
        const idx = row.index;

        return (
          <div>
            <Button
              className="bg-transparent text-foreground  hover:bg-muted-foreground hover:text-foreground"
              onClick={() => showModal(idx, record)}
            >
              <Pencil />
            </Button>
            <Button
              className="bg-transparent text-foreground  hover:bg-muted-foreground hover:text-foreground"
              onClick={() => deleteRecord(idx)}
            >
              <Trash2 />
            </Button>
          </div>
        );
      },
    },
  ];

  const table = useReactTable({
    data,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    state: {
      sorting,
      columnFilters,
      columnVisibility,
    },
  });

  return (
    <div className="rounded-md border w-full">
      <DataTable
        table={table}
        columnsLength={columns.length}
        rootClassName="rounded-md"
        applyColumnCellClassName
      />
    </div>
  );
}
