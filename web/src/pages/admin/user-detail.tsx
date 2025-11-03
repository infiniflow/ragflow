import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'umi';

import { LucideArrowLeft, LucideDot, LucideUser2 } from 'lucide-react';

import { useQuery } from '@tanstack/react-query';
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';

import { cn } from '@/lib/utils';
import { Routes } from '@/routes';

import { Avatar } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
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
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs-underlined';

import {
  getUserDetails,
  listUserAgents,
  listUserDatasets,
} from '@/services/admin-service';

import EnterpriseFeature from './components/enterprise-feature';
import { getSortIcon, parseBooleanish } from './utils';

const ASSET_NAMES = ['dataset', 'flow'];

const datasetColumnHelper =
  createColumnHelper<AdminService.ListUserDatasetItem>();
const agentColumnHelper = createColumnHelper<AdminService.ListUserAgentItem>();

function UserDatasetTable(props: {
  data?: AdminService.ListUserDatasetItem[];
}) {
  const { t } = useTranslation();

  const columnDefs = useMemo(
    () => [
      datasetColumnHelper.accessor('name', {
        header: t('admin.name'),
        enableSorting: false,
      }),
      datasetColumnHelper.accessor('status', {
        header: t('admin.status'),
        cell: ({ cell }) => {
          return (
            <Badge
              variant="secondary"
              className={cn(
                'font-normal text-sm pl-2',
                parseBooleanish(cell.getValue())
                  ? 'bg-state-success-5 text-state-success'
                  : 'bg-state-error-5 text-state-error',
              )}
            >
              <LucideDot className="size-[1em] stroke-[8] mr-1" />
              {t(
                parseBooleanish(cell.getValue())
                  ? 'admin.active'
                  : 'admin.inactive',
              )}
            </Badge>
          );
        },
        enableSorting: false,
      }),
      datasetColumnHelper.accessor('chunk_num', {
        header: t('admin.chunkNum'),
      }),
      datasetColumnHelper.accessor('doc_num', {
        header: t('admin.docNum'),
      }),
      datasetColumnHelper.accessor('token_num', {
        header: t('admin.tokenNum'),
      }),
      datasetColumnHelper.accessor('language', {
        header: t('admin.language'),
        enableSorting: false,
      }),
      datasetColumnHelper.accessor('create_date', {
        header: t('admin.createDate'),
      }),
      datasetColumnHelper.accessor('update_date', {
        header: t('admin.updateDate'),
      }),
      datasetColumnHelper.accessor('permission', {
        header: t('admin.permission'),
        enableSorting: false,
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: props.data ?? [],
    columns: columnDefs,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <section className="space-y-4">
      <Table>
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
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell
                colSpan={table.getAllColumns().length}
                className="h-24 text-center"
              >
                {t('common.noData')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>

      <RAGFlowPagination
        total={props.data?.length}
        current={table.getState().pagination.pageIndex + 1}
        pageSize={table.getState().pagination.pageSize}
        onChange={(page, pageSize) => {
          table.setPagination({
            pageIndex: page - 1,
            pageSize,
          });
        }}
      />
    </section>
  );
}

function UserAgentTable(props: { data?: AdminService.ListUserAgentItem[] }) {
  const { t } = useTranslation();

  const columnDefs = useMemo(
    () => [
      agentColumnHelper.accessor('title', {
        header: t('admin.agentTitle'),
      }),
      agentColumnHelper.accessor('permission', {
        header: t('admin.permission'),
      }),
      agentColumnHelper.accessor('canvas_category', {
        header: t('admin.canvasCategory'),
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: props.data ?? [],
    columns: columnDefs,

    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <section className="space-y-4">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id}>
                  {header.isPlaceholder ? null : (
                    <>
                      {flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                      {/* {header.column.getCanFilter() && (
                            <Button
                              variant="ghost"
                            >
                              <LucideFilter />
                            </Button>
                          )} */}
                    </>
                  )}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>

        <TableBody>
          {table.getRowModel().rows?.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow key="empty">
              <TableCell
                colSpan={table.getAllColumns().length}
                className="h-24 text-center"
              >
                {t('common.noData')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>

      <RAGFlowPagination
        total={props.data?.length}
        current={table.getState().pagination.pageIndex + 1}
        pageSize={table.getState().pagination.pageSize}
        onChange={(page, pageSize) => {
          table.setPagination({
            pageIndex: page - 1,
            pageSize,
          });
        }}
      />
    </section>
  );
}

function AdminUserDetail() {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const { id } = useParams();

  const { data: { detail, datasets, agents } = {} } = useQuery({
    queryKey: ['admin/userDetail', id],
    queryFn: async () => {
      const [userDetails, userDatasets, userAgents] = await Promise.all([
        getUserDetails(id!),
        listUserDatasets(id!),
        listUserAgents(id!),
      ]);

      return {
        detail: userDetails.data.data[0],
        datasets: userDatasets.data.data,
        agents: userAgents.data.data,
      };
    },
    enabled: !!id,
    retry: false,
  });

  return (
    <section className="px-10 py-5 size-full flex flex-col">
      <nav className="mb-5">
        <Button
          variant="outline"
          className="h-10 px-3 dark:bg-bg-input dark:border-border-button"
          onClick={() => navigate(`${Routes.AdminUserManagement}`)}
        >
          <LucideArrowLeft />
          <span>{t('admin.userManagement')}</span>
        </Button>
      </nav>

      <Card className="!shadow-none h-0 basis-0 grow flex flex-col bg-transparent border dark:border-border-button overflow-hidden">
        <CardHeader className="pb-10 border-b dark:border-border-button space-y-8">
          <section className="flex items-center gap-4 text-base">
            <Avatar className="justify-center items-center bg-bg-group uppercase">
              {detail?.email
                .split('@')[0]
                .replace(/[^0-9a-z]/gi, '')
                .slice(0, 2) || <LucideUser2 />}
            </Avatar>

            <span>{detail?.email}</span>

            <Badge
              variant="secondary"
              className={cn(
                'font-normal text-sm pl-2',
                parseBooleanish(detail?.is_active)
                  ? 'bg-state-success-5 text-state-success'
                  : '',
              )}
            >
              <LucideDot className="size-[1em] stroke-[8] mr-1" />
              {t(
                parseBooleanish(detail?.is_active)
                  ? 'admin.active'
                  : 'admin.inactive',
              )}
            </Badge>

            <EnterpriseFeature>
              {() => (
                <Badge variant="secondary" className="font-normal text-sm">
                  {detail?.role}
                </Badge>
              )}
            </EnterpriseFeature>
          </section>

          <section className="flex items-start px-14 space-x-14">
            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.lastLoginTime')}
              </div>
              <div>{detail?.last_login_time}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.createTime')}
              </div>
              <div>{detail?.create_date}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.lastUpdateTime')}
              </div>
              <div>{detail?.update_date}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.language')}
              </div>
              <div>{detail?.language}</div>
            </div>

            <div>
              <div className="text-sm text-text-secondary mb-2">
                {t('admin.isAnonymous')}
              </div>
              <div>{t(detail?.is_anonymous ? 'admin.yes' : 'admin.no')}</div>
            </div>
          </section>
        </CardHeader>

        <CardContent className="h-0 basis-0 grow pt-6">
          <Tabs className="h-full flex flex-col" defaultValue="dataset">
            <TabsList className="p-0 mb-2 gap-4 bg-transparent">
              {ASSET_NAMES.map((name) => (
                <TabsTrigger
                  key={name}
                  className="border-border-button data-[state=active]:bg-bg-card"
                  value={name}
                >
                  {t(`header.${name}`)}
                </TabsTrigger>
              ))}
            </TabsList>

            <TabsContent value="dataset" className="h-0 basis-0 grow">
              <ScrollArea className="h-full">
                <UserDatasetTable data={datasets} />
              </ScrollArea>
            </TabsContent>

            <TabsContent value="flow" className="h-0 basis-0 grow">
              <ScrollArea className="h-full">
                <UserAgentTable data={agents} />
              </ScrollArea>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
    </section>
  );
}

export default AdminUserDetail;
