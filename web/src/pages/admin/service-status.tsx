import { useEffect, useMemo, useState } from 'react';
import { Trans, useTranslation } from 'react-i18next';

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
  LucideClipboardList,
  LucideDot,
  LucideFilter,
  LucideSearch,
  LucideSettings2,
} from 'lucide-react';

import { useQuery } from '@tanstack/react-query';

import { cn } from '@/lib/utils';

import { TableEmpty } from '@/components/table-skeleton';
import { Badge } from '@/components/ui/badge';
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
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

import { listServices, showServiceDetails } from '@/services/admin-service';

import {
  EMPTY_DATA,
  createColumnFilterFn,
  createFuzzySearchFn,
  getSortIcon,
} from './utils';

import ServiceDetail from './service-detail';

const columnHelper = createColumnHelper<AdminService.ListServicesItem>();
const globalFilterFn = createFuzzySearchFn<AdminService.ListServicesItem>([
  'name',
  'service_type',
]);

const SERVICE_TYPE_FILTER_OPTIONS = [
  { value: 'ragflow_server', label: 'ragflow_server' },
  { value: 'meta_data', label: 'meta_data' },
  { value: 'file_store', label: 'file_store' },
  { value: 'retrieval', label: 'retrieval' },
  { value: 'message_queue', label: 'message_queue' },
];

function AdminServiceStatus() {
  const { t } = useTranslation();
  const [extraInfoModalOpen, setExtraInfoModalOpen] = useState(false);
  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [itemToMakeAction, setItemToMakeAction] =
    useState<AdminService.ListServicesItem | null>(null);

  const { data: servicesList } = useQuery({
    queryKey: ['admin/listServices'],
    queryFn: async () => (await listServices()).data.data,
    retry: false,
  });

  const { data: serviceDetails, error: serviceDetailsError } = useQuery({
    queryKey: ['admin/serviceDetails', itemToMakeAction?.id],
    queryFn: async () =>
      (await showServiceDetails(itemToMakeAction!?.id)).data.data,
    enabled: !!(itemToMakeAction && detailModalOpen),
    retry: false,
  });

  const columnDefs = useMemo(
    () => [
      columnHelper.accessor('id', {
        header: t('admin.id'),
      }),
      columnHelper.accessor('name', {
        header: t('admin.name'),
      }),
      columnHelper.accessor('service_type', {
        header: t('admin.serviceType'),
        filterFn: createColumnFilterFn(
          (row, id, filterValue) => row.getValue(id) === filterValue,
          {
            autoRemove: (v) => !v,
            resolveFilterValue: (v) => v || null,
          },
        ),
        enableSorting: false,
      }),
      columnHelper.accessor('host', {
        header: t('admin.host'),
        cell: ({ row }) => (
          <Badge variant="secondary" className="font-normal text-text-primary">
            <i>{row.getValue('host')}</i>
          </Badge>
        ),
      }),
      columnHelper.accessor('port', {
        header: t('admin.port'),
        cell: ({ row }) => (
          <Badge variant="secondary" className="font-normal text-text-primary">
            <i>{row.getValue('port')}</i>
          </Badge>
        ),
      }),
      columnHelper.accessor('status', {
        header: t('admin.status'),
        cell: ({ cell }) => (
          <Badge
            variant="secondary"
            className={cn(
              'pl-2 font-normal text-sm text-text-primary capitalize',
              {
                alive: 'bg-state-success-5 text-state-success',
                timeout: 'bg-state-error-5 text-state-error',
                fail: 'bg-gray-500/5 text-text-disable',
              }[cell.getValue<string>()],
            )}
          >
            <LucideDot className="size-[1em] stroke-[8] mr-1" />
            {cell.getValue()}
          </Badge>
        ),
        enableSorting: false,
      }),
      columnHelper.display({
        id: 'actions',
        header: t('admin.actions'),
        cell: ({ row }) => (
          <div className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity duration-100">
            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setItemToMakeAction(row.original);
                setExtraInfoModalOpen(true);
              }}
            >
              <LucideSettings2 />
            </Button>

            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setItemToMakeAction(row.original);
                setDetailModalOpen(true);
              }}
            >
              <LucideClipboardList />
            </Button>
          </div>
        ),
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: servicesList ?? EMPTY_DATA,
    columns: columnDefs,

    globalFilterFn,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  useEffect(() => {
    if (detailModalOpen && serviceDetailsError) {
      setDetailModalOpen(false);
    }
  }, [detailModalOpen, serviceDetailsError]);

  return (
    <>
      <Card className="!shadow-none h-full border border-border-button bg-transparent rounded-xl">
        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.serviceStatus')}</CardTitle>

            <div className="flex items-center gap-4">
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    size="icon"
                    variant="outline"
                    className="dark:bg-bg-input dark:border-border-button text-text-secondary"
                  >
                    <LucideFilter className="h-4 w-4" />
                  </Button>
                </PopoverTrigger>

                <PopoverContent
                  align="end"
                  className="bg-bg-base text-text-secondary"
                >
                  <div className="p-2 space-y-6">
                    <section>
                      <div className="font-bold mb-3">
                        {t('admin.serviceType')}
                      </div>

                      <RadioGroup
                        value={
                          table
                            .getColumn('service_type')!
                            ?.getFilterValue() as string
                        }
                        onValueChange={
                          table.getColumn('service_type')!?.setFilterValue
                        }
                      >
                        <Label className="space-x-2">
                          <RadioGroupItem
                            className="bg-bg-input border-border-button"
                            value=""
                          />
                          <span>{t('admin.all')}</span>
                        </Label>

                        {SERVICE_TYPE_FILTER_OPTIONS.map(({ label, value }) => (
                          <Label key={value} className="space-x-2">
                            <RadioGroupItem
                              className="bg-bg-input border-border-button"
                              value={value}
                            />
                            <span>{label}</span>
                          </Label>
                        ))}
                      </RadioGroup>
                    </section>
                  </div>

                  <div className="pt-4 flex justify-end">
                    <Button
                      variant="outline"
                      className="dark:bg-bg-input dark:border-border-button text-text-secondary"
                      onClick={() => table.resetColumnFilters()}
                    >
                      {t('admin.reset')}
                    </Button>
                  </div>
                </PopoverContent>
              </Popover>

              <div className="relative w-56">
                <LucideSearch className="absolute left-3 top-1/2 transform -translate-y-1/2 text-gray-400 h-4 w-4" />
                <Input
                  className="pl-10 h-10 bg-bg-input border-border-button"
                  placeholder={t('header.search')}
                  value={table.getState().globalFilter}
                  onChange={(e) => table.setGlobalFilter(e.target.value)}
                />
              </div>
            </div>
          </CardHeader>

          <CardContent>
            <Table>
              <colgroup>
                <col className="w-[6%]" />
                <col />
                <col className="w-[22%]" />
                <col className="w-[13%]" />
                <col className="w-[10%]" />
                <col className="w-[10%]" />
                <col className="w-52" />
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
              total={servicesList?.length}
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

      {/* Extra info modal*/}
      <Dialog open={extraInfoModalOpen} onOpenChange={setExtraInfoModalOpen}>
        <DialogContent
          className="p-0 border-border-button"
          onAnimationEnd={() => {
            if (!extraInfoModalOpen) {
              setItemToMakeAction(null);
            }
          }}
        >
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.extraInfo')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 pt-6 pb-4">
            <div className="rounded-lg p-4 border border-border-button bg-bg-input">
              <pre className="text-sm">
                <code>
                  {JSON.stringify(itemToMakeAction?.extra ?? {}, null, 2)}
                </code>
              </pre>
            </div>
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setExtraInfoModalOpen(false)}
            >
              {t('admin.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Service details modal */}
      <Dialog open={detailModalOpen} onOpenChange={setDetailModalOpen}>
        <DialogContent
          className="flex flex-col max-h-[calc(100vh-4rem)] max-w-6xl p-0 border-border-button"
          onAnimationEnd={() => {
            if (!detailModalOpen) {
              setItemToMakeAction(null);
            }
          }}
        >
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>
              <Trans i18nKey="admin.serviceDetail">
                {{ name: itemToMakeAction?.name }}
              </Trans>
            </DialogTitle>
          </DialogHeader>

          <ScrollArea className="pt-6 pb-4 px-12 h-0 flex-1 text-text-secondary flex flex-col">
            <ServiceDetail content={serviceDetails?.message} />
          </ScrollArea>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setDetailModalOpen(false);
              }}
            >
              {t('admin.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminServiceStatus;
