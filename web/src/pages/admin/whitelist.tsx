import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import * as XLSX from 'xlsx';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';

import {
  LucideDownload,
  LucidePlus,
  LucideSearch,
  LucideTrash2,
  LucideUpload,
  LucideUserPen,
} from 'lucide-react';

import Spotlight from '@/components/spotlight';
import { TableEmpty } from '@/components/table-skeleton';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';

import {
  createWhitelistEntry,
  deleteWhitelistEntry,
  importWhitelistFromExcel,
  listWhitelist,
  updateWhitelistEntry,
} from '@/services/admin-service';

import { EMPTY_DATA, createFuzzySearchFn, getSortIcon } from './utils';

import dayjs from 'dayjs';
import useCreateEmailForm from './forms/email-form';
import useImportExcelForm, {
  ImportExcelFormData,
} from './forms/import-excel-form';

const columnHelper = createColumnHelper<AdminService.ListWhitelistItem>();
const globalFilterFn = createFuzzySearchFn<AdminService.ListWhitelistItem>([
  'email',
]);

function AdminWhitelist() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const createEmailForm = useCreateEmailForm();
  const editEmailForm = useCreateEmailForm();
  const importExcelForm = useImportExcelForm();

  const [itemToMakeAction, setItemToMakeAction] =
    useState<AdminService.ListWhitelistItem | null>(null);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [editModalOpen, setEditModalOpen] = useState(false);

  const [importModalOpen, setImportModalOpen] = useState(false);

  const { data: whitelist } = useQuery({
    queryKey: ['admin/listWhitelist'],
    queryFn: async () => (await listWhitelist())?.data?.data?.white_list,
    retry: false,
  });

  // Reset form when editing a different email
  useEffect(() => {
    if (itemToMakeAction && editModalOpen) {
      editEmailForm.form.setValue('email', itemToMakeAction.email);
    }
  }, [itemToMakeAction, editModalOpen, editEmailForm.form]);

  const createWhitelistEntryMutation = useMutation({
    mutationFn: (data: { email: string }) => createWhitelistEntry(data.email),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listWhitelist'] });
      setCreateModalOpen(false);
      createEmailForm.form.reset();
    },
    onError: (error) => {
      console.error('Error creating email:', error);
    },
    retry: false,
  });

  const updateWhitelistEntryMutation = useMutation({
    mutationFn: (data: { id: number; email: string }) =>
      updateWhitelistEntry(data.id, data.email),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listWhitelist'] });
      setEditModalOpen(false);
      setItemToMakeAction(null);
      editEmailForm.form.reset();
    },
  });

  const deleteWhitelistEntryMutation = useMutation({
    mutationFn: (data: { email: string }) => deleteWhitelistEntry(data.email),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listWhitelist'] });
      setDeleteModalOpen(false);
      setItemToMakeAction(null);
    },
    onError: (error) => {
      console.error('Error deleting email:', error);
    },
  });

  const importExcelMutation = useMutation({
    mutationFn: (data: ImportExcelFormData) =>
      importWhitelistFromExcel(data.file),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listWhitelist'] });
      setImportModalOpen(false);
      importExcelForm.form.reset();
    },
    onError: (error) => {
      console.error('Error importing Excel:', error);
    },
    retry: false,
  });

  const handleExportExcel = () => {
    const columnData = (whitelist ?? EMPTY_DATA).map((item) => ({
      email: item.email,
    }));

    const worksheet = XLSX.utils.json_to_sheet(columnData);
    const workbook = XLSX.utils.book_new();

    XLSX.utils.book_append_sheet(workbook, worksheet, 'Sheet1');
    XLSX.writeFile(
      workbook,
      `whitelist_${dayjs(new Date()).format('YYYYMMDDHHmmss')}.xlsx`,
    );
  };

  const columnDefs = useMemo(
    () => [
      columnHelper.accessor('email', {
        header: t('admin.email'),
        enableSorting: false,
      }),
      columnHelper.accessor('create_date', {
        header: t('admin.createDate'),
      }),
      columnHelper.accessor('update_date', {
        header: t('admin.updateDate'),
      }),
      columnHelper.display({
        id: 'actions',
        header: t('admin.actions'),
        cell: ({ row }) => (
          <div className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity">
            <Button
              variant="transparent"
              size="icon"
              className="border-0"
              onClick={() => {
                setItemToMakeAction(row.original);
                setEditModalOpen(true);
              }}
            >
              <LucideUserPen />
            </Button>
            <Button
              variant="danger"
              size="icon"
              className="border-0"
              onClick={() => {
                setItemToMakeAction(row.original);
                setDeleteModalOpen(true);
              }}
            >
              <LucideTrash2 />
            </Button>
          </div>
        ),
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: whitelist ?? EMPTY_DATA,
    columns: columnDefs,

    globalFilterFn,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),

    enableSorting: false,
  });

  return (
    <>
      <Card className="!shadow-none relative h-full border-0.5 border-border-button bg-transparent rounded-xl overflow-x-hidden overflow-y-auto">
        <Spotlight />

        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.whitelistManagement')}</CardTitle>

            <div className="flex items-center gap-4">
              <SearchInput
                className="w-56 h-10 bg-bg-input border-border-button"
                placeholder={t('header.search')}
                value={table.getState().globalFilter}
                onChange={(e) => table.setGlobalFilter(e.target.value)}
                prefix={<LucideSearch className="size-3.5" />}
              />

              <Button
                variant="outline"
                className="h-10 px-4 dark:bg-bg-input dark:border-border-button text-text-secondary"
                onClick={handleExportExcel}
              >
                <LucideUpload />
                {t('admin.exportAsExcel')}
              </Button>

              <Button
                variant="outline"
                className="h-10 px-4 dark:bg-bg-input dark:border-border-button text-text-secondary"
                onClick={() => setImportModalOpen(true)}
              >
                <LucideDownload />
                {t('admin.importFromExcel')}
              </Button>

              <Button
                className="h-10 px-4"
                onClick={() => setCreateModalOpen(true)}
              >
                <LucidePlus />
                {t('admin.newUser')}
              </Button>
            </div>
          </CardHeader>

          <CardContent>
            <Table>
              <colgroup>
                <col />
                <col className="w-[25%]" />
                <col className="w-[25%]" />
                <col className="w-[12rem]" />
              </colgroup>

              <TableHeader>
                {table.getHeaderGroups().map((headerGroup) => (
                  <TableRow key={headerGroup.id}>
                    {headerGroup.headers.map((header) => (
                      <TableHead key={header.id}>
                        {header.isPlaceholder ? null : header.column.getCanSort() ? (
                          <Button
                            variant="ghost"
                            onClick={header.column.getToggleSortingHandler()}
                          >
                            {flexRender(
                              header.column.columnDef.header,
                              header.getContext(),
                            )}
                            {getSortIcon(header.column.getIsSorted())}
                          </Button>
                        ) : (
                          flexRender(
                            header.column.columnDef.header,
                            header.getContext(),
                          )
                        )}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {table.getRowModel().rows?.length ? (
                  table.getRowModel().rows.map((row) => (
                    <TableRow key={row.id} className="group">
                      {row.getVisibleCells().map((cell) => (
                        <TableCell key={cell.id}>
                          {flexRender(
                            cell.column.columnDef.cell,
                            cell.getContext(),
                          )}
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : (
                  <TableEmpty columnsLength={columnDefs.length} />
                )}
              </TableBody>
            </Table>
          </CardContent>

          <CardFooter className="flex items-center justify-end">
            <RAGFlowPagination
              total={table.getFilteredRowModel().rows.length}
              current={table.getState().pagination.pageIndex + 1}
              pageSize={table.getState().pagination.pageSize}
              onChange={(page, pageSize) => {
                table.setPagination({
                  pageIndex: page - 1,
                  pageSize,
                });
              }}
            />
          </CardFooter>
        </ScrollArea>
      </Card>

      {/* Delete Confirmation Modal */}
      <Dialog open={deleteModalOpen} onOpenChange={setDeleteModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!deleteModalOpen) {
              setItemToMakeAction(null);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.deleteEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <DialogDescription className="text-text-primary">
              {t('admin.deleteWhitelistEmailConfirmation')}
            </DialogDescription>

            <div className="rounded-lg mt-6 p-4 border-0.5 border-border-button">
              {itemToMakeAction?.email}
            </div>
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={deleteWhitelistEntryMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              className="px-4 h-10"
              variant="destructive"
              onClick={() => {
                if (itemToMakeAction) {
                  deleteWhitelistEntryMutation.mutate({
                    email: itemToMakeAction?.email,
                  });
                }
              }}
              disabled={deleteWhitelistEntryMutation.isPending}
              loading={deleteWhitelistEntryMutation.isPending}
            >
              {t('admin.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Email Modal */}
      <Dialog open={createModalOpen} onOpenChange={setCreateModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!createModalOpen) {
              createEmailForm.form.reset();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.createEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <createEmailForm.FormComponent
              id={createEmailForm.id}
              onSubmit={createWhitelistEntryMutation.mutate}
            />
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setCreateModalOpen(false)}
              disabled={createWhitelistEntryMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              form={createEmailForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={createWhitelistEntryMutation.isPending}
              loading={createWhitelistEntryMutation.isPending}
            >
              {t('admin.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Email Modal */}
      <Dialog open={editModalOpen} onOpenChange={setEditModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!editModalOpen) {
              setItemToMakeAction(null);
              editEmailForm.form.reset();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.editEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <editEmailForm.FormComponent
              id={editEmailForm.id}
              onSubmit={(value) => {
                if (itemToMakeAction) {
                  updateWhitelistEntryMutation.mutate({
                    id: itemToMakeAction.id,
                    email: value.email,
                  });
                }
              }}
            />
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setEditModalOpen(false)}
              disabled={updateWhitelistEntryMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              form={editEmailForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={updateWhitelistEntryMutation.isPending}
              loading={updateWhitelistEntryMutation.isPending}
            >
              {t('admin.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import Excel Modal */}
      <Dialog open={importModalOpen} onOpenChange={setImportModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!importModalOpen) {
              importExcelForm.form.reset();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.importWhitelist')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <importExcelForm.FormComponent
              id={importExcelForm.id}
              onSubmit={importExcelMutation.mutate}
            />
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setImportModalOpen(false)}
              disabled={importExcelMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              form={importExcelForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={importExcelMutation.isPending}
              loading={importExcelMutation.isPending}
            >
              {t('admin.import')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminWhitelist;
