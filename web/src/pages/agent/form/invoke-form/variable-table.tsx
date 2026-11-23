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
import { useGetVariableLabelOrTypeByValue } from '../../hooks/use-get-begin-query';
import { VariableFormSchemaType } from './schema';

interface IProps {
  data: VariableFormSchemaType[];
  deleteRecord(index: number): void;
  showModal(index: number, record: VariableFormSchemaType): void;
  nodeId?: string;
}

export function VariableTable({
  data = [],
  deleteRecord,
  showModal,
  nodeId,
}: IProps) {
  const { t } = useTranslation();
  const { getLabel } = useGetVariableLabelOrTypeByValue({ nodeId: nodeId! });

  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});

  const columns: ColumnDef<VariableFormSchemaType>[] = [
    {
      accessorKey: 'key',
      header: t('flow.key'),
      meta: { cellClassName: 'max-w-30' },
      cell: ({ row }) => {
        const key: string = row.getValue('key');
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="truncate">{key}</div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{key}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'type',
      header: t('flow.type'),
      cell: ({ row }) => (
        <div>{row.original.type === 'object' ? 'Object' : 'String'}</div>
      ),
    },
    {
      accessorKey: 'ref',
      header: t('flow.ref'),
      meta: { cellClassName: 'max-w-30' },
      cell: ({ row }) => {
        const ref: string = row.getValue('ref');
        const label = getLabel(ref);
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="truncate">{label}</div>
            </TooltipTrigger>
            <TooltipContent>
              <p>{label}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
    {
      accessorKey: 'value',
      header: t('flow.value'),
      cell: ({ row }) => <div>{row.getValue('value')}</div>,
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
    <div className="w-full">
      <div className="rounded-md border">
        <DataTable
          table={table}
          columnsLength={columns.length}
          rootClassName="rounded-md"
          applyColumnCellClassName
        />
      </div>
    </div>
  );
}
