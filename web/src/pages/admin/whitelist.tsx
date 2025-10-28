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
import { Input } from '@/components/ui/input';
import { LoadingButton } from '@/components/ui/loading-button';
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
import { useMutation, useQueryClient } from '@tanstack/react-query';
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
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import useCreateEmailForm from './forms/email-form';
import useImportExcelForm from './forms/import-excel-form';
import { EMPTY_DATA, createFuzzySearchFn } from './utils';

// #region FAKE DATA
function _pickRandom<T extends unknown>(arr: T[]): T | void {
  return arr[Math.floor(Math.random() * arr.length)];
}

const PSEUDO_TABLE_ITEMS = Array.from({ length: 20 }, () => ({
  id: Math.random().toString(36).slice(2, 8),
  email: `${Math.random().toString(36).slice(2, 6)}@example.com`,
  created_by: _pickRandom(['Alice', 'Bob', 'Carol', 'Dave']) || 'System',
  created_at: Date.now() - Math.floor(Math.random() * 1000 * 60 * 60 * 24 * 30),
}));
// #endregion

const columnHelper = createColumnHelper<(typeof PSEUDO_TABLE_ITEMS)[0]>();
const globalFilterFn = createFuzzySearchFn<(typeof PSEUDO_TABLE_ITEMS)[0]>([
  'email',
]);

function AdminWhitelist() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const createEmailForm = useCreateEmailForm();
  const importExcelForm = useImportExcelForm();

  const [emailToMakeAction, setEmailToMakeAction] = useState<string | null>(
    null,
  );
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [editModalOpen, setEditModalOpen] = useState(false);

  const [importModalOpen, setImportModalOpen] = useState(false);

  // Reset form when editing a different email
  useEffect(() => {
    if (emailToMakeAction && editModalOpen) {
      createEmailForm.form.setValue('email', emailToMakeAction);
    }
  }, [emailToMakeAction, editModalOpen, createEmailForm.form]);

  const { isPending: isCreating, mutateAsync: createEmail } = useMutation({
    mutationFn: async (data: { email: string }) => {
      /* create email API call */
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/whitelist'] });
      setCreateModalOpen(false);
      setEmailToMakeAction(null);
      createEmailForm.form.reset();
    },
    onError: (error) => {
      console.error('Error creating email:', error);
    },
  });

  const { isPending: isEditing, mutateAsync: updateEmail } = useMutation({
    mutationFn: async (data: { email: string }) => {
      /* update email API call */
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/whitelist'] });
      setEditModalOpen(false);
      setEmailToMakeAction(null);
      createEmailForm.form.reset();
    },
  });

  const { isPending: isDeleting, mutateAsync: deleteEmail } = useMutation({
    mutationFn: async (data: { email: string }) => {
      /* delete email API call */
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/whitelist'] });
      setDeleteModalOpen(false);
      setEmailToMakeAction(null);
    },
    onError: (error) => {
      console.error('Error deleting email:', error);
    },
  });

  const { isPending: isImporting, mutateAsync: importExcel } = useMutation({
    mutationFn: async (data: {
      file: FileList;
      overwriteExisting: boolean;
    }) => {
      /* import Excel API call */
      console.log(
        'Importing Excel file:',
        data.file[0]?.name,
        'Overwrite:',
        data.overwriteExisting,
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/whitelist'] });
      setImportModalOpen(false);
      importExcelForm.form.reset();
    },
    onError: (error) => {
      console.error('Error importing Excel:', error);
    },
  });

  const columnDefs = useMemo(
    () => [
      columnHelper.accessor('email', {
        header: 'Email',
      }),
      columnHelper.accessor('created_by', {
        header: 'Created by',
      }),
      columnHelper.accessor('created_at', {
        header: 'Created date',
        cell: ({ row }) =>
          new Date(row.getValue('created_at') as number).toLocaleString(),
      }),
      columnHelper.display({
        id: 'actions',
        header: 'Actions',
        cell: ({ row }) => (
          <div className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-100">
            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setEmailToMakeAction(row.original.email);
                setEditModalOpen(true);
              }}
            >
              <LucideUserPen />
            </Button>
            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setEmailToMakeAction(row.original.email);
                setDeleteModalOpen(true);
              }}
            >
              <LucideTrash2 />
            </Button>
          </div>
        ),
      }),
    ],
    [],
  );

  const table = useReactTable({
    data: PSEUDO_TABLE_ITEMS ?? EMPTY_DATA,
    columns: columnDefs,

    globalFilterFn,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  return (
    <>
      <Card className="h-full border border-border-button bg-transparent rounded-xl overflow-x-hidden overflow-y-auto">
        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.whitelistManagement')}</CardTitle>

            <div className="flex items-center gap-4">
              <div className="relative w-56">
                <LucideSearch className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
                <Input
                  className="pl-10 h-10 bg-bg-input border-border-button"
                  placeholder={t('header.search')}
                  value={table.getState().globalFilter}
                  onChange={(e) => table.setGlobalFilter(e.target.value)}
                />
              </div>

              <Button
                variant="outline"
                className="h-10 px-4 dark:bg-bg-input dark:border-border-button text-text-secondary"
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
                {t('admin.createEmail')}
              </Button>
            </div>
          </CardHeader>

          <CardContent>
            <Table>
              <colgroup>
                <col />
                <col className="w-[20%]" />
                <col className="w-[30%]" />
                <col className="w-[12rem]" />
              </colgroup>

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
          className="p-0 border-border-button"
          onAnimationEnd={() => {
            if (!deleteModalOpen) {
              setEmailToMakeAction(null);
            }
          }}
        >
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.deleteEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4">
            <DialogDescription className="text-text-primary">
              {t('admin.deleteWhitelistEmailConfirmation')}

              <div className="rounded-lg mt-6 p-4 border border-border-button">
                {emailToMakeAction}
              </div>
            </DialogDescription>
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={isDeleting}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              className="px-4 h-10"
              variant="destructive"
              onClick={() => {
                deleteEmail({ email: emailToMakeAction! });
              }}
              disabled={isDeleting}
              loading={isDeleting}
            >
              {t('admin.delete')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create Email Modal */}
      <Dialog
        open={createModalOpen}
        onOpenChange={() => {
          setCreateModalOpen(false);
          createEmailForm.form.reset();
        }}
      >
        <DialogContent className="p-0 border-border-button">
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.createEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4 text-text-secondary">
            <createEmailForm.FormComponent
              id={createEmailForm.id}
              onSubmit={createEmail}
            />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setCreateModalOpen(false);
                createEmailForm.form.reset();
              }}
              disabled={isCreating}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              form={createEmailForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={isCreating}
              loading={isCreating}
            >
              {t('admin.confirm')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Email Modal */}
      <Dialog
        open={editModalOpen}
        onOpenChange={() => {
          setEditModalOpen(false);
          setEmailToMakeAction(null);
          createEmailForm.form.reset();
        }}
      >
        <DialogContent
          className="p-0 border-border-button"
          onAnimationEnd={() => {
            if (!editModalOpen) {
              setEmailToMakeAction(null);
              createEmailForm.form.reset();
            }
          }}
        >
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.editEmail')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4 text-text-secondary">
            <createEmailForm.FormComponent
              id={createEmailForm.id}
              onSubmit={updateEmail}
            />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setEditModalOpen(false);
                setEmailToMakeAction(null);
                createEmailForm.form.reset();
              }}
              disabled={isEditing}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              form={createEmailForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={isEditing}
              loading={isEditing}
            >
              {t('admin.confirm')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Import Excel Modal */}
      <Dialog open={importModalOpen} onOpenChange={setImportModalOpen}>
        <DialogContent className="p-0 border-border-button">
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.importWhitelist')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4 text-text-secondary">
            <importExcelForm.FormComponent
              id={importExcelForm.id}
              onSubmit={importExcel}
            />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setImportModalOpen(false);
                importExcelForm.form.reset();
              }}
              disabled={isImporting}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              form={importExcelForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={isImporting}
              loading={isImporting}
            >
              {t('admin.import')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminWhitelist;
