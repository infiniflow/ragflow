import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { EmptyType } from '@/components/empty/constant';
import Empty from '@/components/empty/empty';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Pagination } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { replaceText } from '@/pages/dataset/process-log-modal';
import { MemoryOptions } from '@/pages/memories/constants';
import {
  ColumnDef,
  ColumnFiltersState,
  ExpandedState,
  Row,
  SortingState,
  VisibilityState,
  flexRender,
  getCoreRowModel,
  getExpandedRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import { t } from 'i18next';
import { pick } from 'lodash';
import {
  Copy,
  Eraser,
  ListChevronsDownUp,
  ListChevronsUpDown,
  TextSelect,
} from 'lucide-react';
import * as React from 'react';
import { useMemo, useState } from 'react';
import { CopyToClipboard } from 'react-copy-to-clipboard';
import { useMessageAction } from './hook';
import { IMessageInfo } from './interface';

export type MemoryTableProps = {
  messages: Array<IMessageInfo>;
  total: number;
  pagination: Pagination;
  setPagination: (params: { page: number; pageSize: number }) => void;
};

export function MemoryTable({
  messages,
  total,
  pagination,
  setPagination,
}: MemoryTableProps) {
  const [sorting, setSorting] = React.useState<SortingState>([]);
  const [columnFilters, setColumnFilters] = React.useState<ColumnFiltersState>(
    [],
  );
  const [columnVisibility, setColumnVisibility] =
    React.useState<VisibilityState>({});
  const [copied, setCopied] = useState(false);
  const {
    showDeleteDialog,
    setShowDeleteDialog,
    handleClickDeleteMessage,
    selectedMessage,
    handleDeleteMessage,

    handleClickUpdateMessageState,
    selectedMessageContent,
    showMessageContentDialog,
    setShowMessageContentDialog,
    handleClickMessageContentDialog,
  } = useMessageAction();

  const disabledRowFunc = (row: Row<IMessageInfo>) => {
    return row.original.forget_at !== 'None' && !!row.original.forget_at;
  };
  // Define columns for the memory table
  const columns: ColumnDef<IMessageInfo>[] = useMemo(
    () => [
      {
        accessorKey: 'session_id',
        header: ({ table }) => (
          <div className="flex items-center gap-1">
            <button
              {...{
                onClick: table.getToggleAllRowsExpandedHandler(),
              }}
            >
              {table.getIsAllRowsExpanded() ? (
                <ListChevronsDownUp size={16} />
              ) : (
                <ListChevronsUpDown size={16} />
              )}
            </button>{' '}
            <span>{t('memory.messages.sessionId')}</span>
          </div>
        ),
        cell: ({ row }) => (
          <div className="flex items-center gap-1">
            {row.getCanExpand() ? (
              <button
                {...{
                  onClick: row.getToggleExpandedHandler(),
                  style: { cursor: 'pointer' },
                }}
              >
                {row.getIsExpanded() ? (
                  <ListChevronsDownUp size={16} />
                ) : (
                  <ListChevronsUpDown size={16} />
                )}
              </button>
            ) : (
              ''
            )}
            <div
              className={cn('text-sm font-medium', {
                'pl-5': !row.getCanExpand(),
              })}
            >
              {row.getValue('session_id')}
            </div>
          </div>
        ),
      },
      {
        accessorKey: 'agent_name',
        header: () => <span>{t('memory.messages.agent')}</span>,
        cell: ({ row }) => (
          <div className="text-sm font-medium ">
            {row.getValue('agent_name')}
          </div>
        ),
      },
      {
        accessorKey: 'message_type',
        header: () => <span>{t('memory.messages.type')}</span>,
        cell: ({ row }) => (
          <div className="text-sm font-medium  capitalize">
            {row.getValue('message_type')
              ? MemoryOptions(t).find(
                  (item) =>
                    item.value === (row.getValue('message_type') as string),
                )?.label
              : row.getValue('message_type')}
          </div>
        ),
      },
      {
        accessorKey: 'valid_at',
        header: () => <span>{t('memory.messages.validDate')}</span>,
        cell: ({ row }) => (
          <div className="text-sm ">{row.getValue('valid_at')}</div>
        ),
      },
      {
        accessorKey: 'forget_at',
        header: () => <span>{t('memory.messages.forgetAt')}</span>,
        cell: ({ row }) => (
          <div className="text-sm ">{row.getValue('forget_at')}</div>
        ),
      },
      // {
      //   accessorKey: 'source_id',
      //   header: () => <span>{t('memory.messages.source')}</span>,
      //   cell: ({ row }) => (
      //     <div className="text-sm ">{row.getValue('source_id')}</div>
      //   ),
      // },
      {
        accessorKey: 'status',
        header: () => <span>{t('memory.messages.enable')}</span>,
        cell: ({ row }) => {
          const isEnabled = row.getValue('status') as boolean;
          return (
            <div className="flex items-center">
              <Switch
                disabled={disabledRowFunc(row)}
                defaultChecked={isEnabled}
                onCheckedChange={(val) => {
                  handleClickUpdateMessageState(row.original, val);
                }}
              />
            </div>
          );
        },
      },
      {
        accessorKey: 'action',
        header: () => <span>{t('memory.messages.action')}</span>,
        meta: {
          cellClassName: 'w-12',
        },
        cell: ({ row }) => (
          <div className=" flex opacity-0 group-hover:opacity-100">
            <Button
              variant={'ghost'}
              className="bg-transparent"
              onClick={() => {
                handleClickMessageContentDialog(row.original);
              }}
            >
              <TextSelect />
            </Button>
            <Button
              variant={'delete'}
              disabled={disabledRowFunc(row)}
              className="bg-transparent"
              aria-label="Edit"
              onClick={() => {
                handleClickDeleteMessage(row.original);
              }}
            >
              <Eraser />
            </Button>
          </div>
        ),
      },
    ],
    [handleClickDeleteMessage],
  );

  const currentPagination = useMemo(() => {
    return {
      pageIndex: (pagination.current || 1) - 1,
      pageSize: pagination.pageSize || 10,
    };
  }, [pagination]);
  const [expanded, setExpanded] = React.useState<ExpandedState>({});
  const table = useReactTable({
    data: messages,
    columns,
    onExpandedChange: setExpanded,
    getSubRows: (row) => row.extract || undefined,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getExpandedRowModel: getExpandedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    manualPagination: true,
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      pagination: currentPagination,
      expanded,
    },
    rowCount: total,
  });

  return (
    <div className="w-full">
      <Table rootClassName="max-h-[calc(100vh-292px)]">
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder
                    ? null
                    : flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody className="relative">
          {table.getRowModel().rows?.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow
                key={row.id}
                data-state={row.getIsSelected() && 'selected'}
                className={cn('group', {
                  'bg-bg-list/5': !row.getCanExpand(),
                })}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-24 text-center">
                <Empty type={EmptyType.Data} />
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      {showDeleteDialog && (
        <ConfirmDeleteDialog
          onOk={handleDeleteMessage}
          title={t('memory.messages.forgetMessage')}
          open={showDeleteDialog}
          onOpenChange={setShowDeleteDialog}
          okButtonText={t('memory.messages.forget')}
          content={{
            title: t('memory.messages.forgetMessageTip'),
            node: (
              <ConfirmDeleteDialogNode
                // avatar={{ avatar: selectedMessage.avatar, name: selectedMessage.name }}
                name={
                  t('memory.messages.sessionId') +
                  ': ' +
                  selectedMessage.session_id
                }
                warnText={t('memory.messages.delMessageWarn')}
              />
            ),
          }}
        />
      )}

      {showMessageContentDialog && (
        <Modal
          title={t('memory.messages.content')}
          open={showMessageContentDialog}
          onOpenChange={setShowMessageContentDialog}
          className="!w-[640px]"
          footer={
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setShowMessageContentDialog(false)}
                className={
                  'px-2 py-1 border border-border-button rounded-md hover:bg-bg-card hover:text-text-primary '
                }
              >
                {t('common.close')}
              </button>
            </div>
          }
        >
          <div className="flex flex-col gap-2.5">
            <div className="text-text-secondary text-sm">
              {t('memory.messages.sessionId')}:&nbsp;&nbsp;
              {selectedMessage.session_id}
            </div>
            {selectedMessageContent?.content && (
              <div className="w-full bg-accent-primary-5  whitespace-pre-line text-wrap rounded-lg h-fit max-h-[350px] overflow-y-auto scrollbar-auto px-2.5 py-1">
                {replaceText(selectedMessageContent?.content || '')}
              </div>
            )}
            {selectedMessageContent?.content_embed && (
              <div className="flex gap-2 items-center">
                <CopyToClipboard
                  text={selectedMessageContent?.content_embed}
                  onCopy={() => {
                    setCopied(true);
                    setTimeout(() => setCopied(false), 1000);
                  }}
                >
                  <Button
                    variant={'ghost'}
                    className="border border-border-button "
                  >
                    {t('memory.messages.contentEmbed')}
                    <Copy />
                  </Button>
                </CopyToClipboard>
                {copied && (
                  <span className="text-xs text-text-secondary">
                    {t('memory.messages.copied')}
                  </span>
                )}
              </div>
            )}
          </div>
        </Modal>
      )}

      <div className="flex items-center justify-end  absolute bottom-3 right-3">
        <RAGFlowPagination
          {...pick(pagination, 'current', 'pageSize')}
          total={total}
          onChange={(page, pageSize) => {
            setPagination({ page, pageSize });
          }}
        />
      </div>
    </div>
  );
}
