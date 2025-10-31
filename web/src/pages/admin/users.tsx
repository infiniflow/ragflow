import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';

import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  LucideClipboardList,
  LucideDot,
  LucideTrash2,
  LucideUserLock,
  LucideUserPlus,
} from 'lucide-react';

import { cn } from '@/lib/utils';
import { rsaPsw } from '@/utils';

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
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { LoadingButton } from '@/components/ui/loading-button';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Routes } from '@/routes';
import { LucideFilter, LucideSearch } from 'lucide-react';

import useChangePasswordForm from './forms/change-password-form';
import useCreateUserForm from './forms/user-form';

import {
  createUser,
  deleteUser,
  listRoles,
  listUsers,
  updateUserPassword,
  updateUserRole,
  updateUserStatus,
  type AdminService,
} from '@/services/admin-service';

import {
  createColumnFilterFn,
  createFuzzySearchFn,
  EMPTY_DATA,
  IS_ENTERPRISE,
  parseBooleanish,
} from './utils';

import EnterpriseFeature from './components/enterprise-feature';

const columnHelper = createColumnHelper<AdminService.ListUsersItem>();
const globalFilterFn = createFuzzySearchFn<AdminService.ListUsersItem>([
  'email',
  'nickname',
]);

const STATUS_FILTER_OPTIONS = [
  { value: '', label: 'admin.all' },
  { value: 'active', label: 'admin.active' },
  { value: 'inactive', label: 'admin.inactive' },
];

function AdminUserManagement() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);
  const [createUserModalOpen, setCreateUserModalOpen] = useState(false);
  const [userToMakeAction, setUserToMakeAction] =
    useState<AdminService.ListUsersItem | null>(null);

  const changePasswordForm = useChangePasswordForm();
  const createUserForm = useCreateUserForm();

  const { data: roleList } = useQuery({
    queryKey: ['admin/listRoles'],
    queryFn: async () => (await listRoles()).data.data.roles,
    enabled: IS_ENTERPRISE,
    retry: false,
  });

  const { data: usersList, isPending } = useQuery({
    queryKey: ['admin/listUsers'],
    queryFn: async () => (await listUsers()).data.data,
    retry: false,
  });

  // Delete user mutation
  const deleteUserMutation = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => {
      // message.success(t('admin.userDeletedSuccessfully'));
      queryClient.invalidateQueries({ queryKey: ['admin/listUsers'] });
      setDeleteModalOpen(false);
      setUserToMakeAction(null);
    },
    retry: false,
  });

  // Change password mutation
  const changePasswordMutation = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      updateUserPassword(email, rsaPsw(password) as string),
    onSuccess: () => {
      // message.success(t('admin.passwordChangedSuccessfully'));
      setPasswordModalOpen(false);
      setUserToMakeAction(null);
    },
    retry: false,
  });

  // Update user role mutation
  const updateUserRoleMutation = useMutation({
    mutationFn: ({ email, role }: { email: string; role: string }) =>
      updateUserRole(email, role),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listUsers'] });
    },
    retry: false,
  });

  // Create user mutation
  const createUserMutation = useMutation({
    mutationFn: async ({
      email,
      password,
      role,
    }: {
      email: string;
      password: string;
      role?: string;
    }) => {
      await createUser(email, rsaPsw(password) as string);

      if (IS_ENTERPRISE && role) {
        await updateUserRoleMutation.mutateAsync({ email, role });
      }
    },
    onSuccess: () => {
      // message.success(t('admin.userCreatedSuccessfully'));
      queryClient.invalidateQueries({ queryKey: ['admin/listUsers'] });
      setCreateUserModalOpen(false);
      createUserForm.form.reset();
    },
    retry: false,
  });

  // Update user status mutation
  const updateUserStatusMutation = useMutation({
    mutationFn: (data: { email: string; isActive: boolean }) =>
      updateUserStatus(data.email, data.isActive ? 'on' : 'off'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listUsers'] });
    },
    retry: false,
  });

  const columnDefs = useMemo(
    () => [
      columnHelper.accessor('email', {
        header: t('admin.email'),
      }),
      columnHelper.accessor('nickname', {
        header: t('admin.nickname'),
      }),

      ...(IS_ENTERPRISE
        ? [
            columnHelper.accessor('role', {
              header: t('admin.role'),
              cell: ({ row, cell }) => (
                <Select
                  value={cell.getValue()}
                  onValueChange={(value) => {
                    if (!updateUserRoleMutation.isPending) {
                      updateUserRoleMutation.mutate({
                        email: row.original.email,
                        role: value,
                      });
                    }
                  }}
                  disabled={updateUserRoleMutation.isPending}
                >
                  <SelectTrigger className="h-10">
                    <SelectValue />
                  </SelectTrigger>

                  <SelectContent className="bg-bg-base">
                    {roleList?.map(({ id, role_name }) => (
                      <SelectItem key={id} value={role_name}>
                        {role_name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ),
              filterFn: createColumnFilterFn(
                (row, id, filterValue) => row.getValue(id) === filterValue,
                {
                  autoRemove: (v) => !v,
                },
              ),
            }),
          ]
        : []),

      columnHelper.display({
        id: 'enable',
        header: t('admin.enable'),
        cell: ({ row }) => (
          <Switch
            checked={parseBooleanish(row.original.is_active)}
            onCheckedChange={(checked) => {
              updateUserStatusMutation.mutate({
                email: row.original.email,
                isActive: checked,
              });
            }}
            disabled={updateUserStatusMutation.isPending}
          />
        ),
      }),
      columnHelper.accessor('is_active', {
        header: t('admin.status'),
        cell: ({ cell }) => (
          <Badge
            variant="secondary"
            className={cn(
              'pl-2 font-normal text-sm',
              parseBooleanish(cell.getValue())
                ? 'bg-state-success-5 text-state-success'
                : '',
            )}
          >
            <LucideDot className="size-[1em] stroke-[8] mr-1" />
            {t(
              parseBooleanish(cell.getValue())
                ? 'admin.active'
                : 'admin.inactive',
            )}
          </Badge>
        ),
        filterFn: createColumnFilterFn(
          (row, id, filterValue) => row.getValue(id) === filterValue,
          {
            autoRemove: (v) => !v,
            resolveFilterValue: (v) =>
              v ? (v === 'active' ? '1' : '0') : null,
          },
        ),
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
              onClick={() =>
                navigate(`${Routes.AdminUserManagement}/${row.original.email}`)
              }
            >
              <LucideClipboardList />
            </Button>
            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setUserToMakeAction(row.original);
                setPasswordModalOpen(true);
              }}
            >
              <LucideUserLock />
            </Button>
            <Button
              variant="transparent"
              size="icon"
              className="border-0 text-text-secondary"
              onClick={() => {
                setUserToMakeAction(row.original);
                setDeleteModalOpen(true);
              }}
            >
              <LucideTrash2 />
            </Button>
          </div>
        ),
      }),
    ],
    [
      roleList,
      t,
      navigate,
      updateUserStatusMutation.isPending,
      updateUserRoleMutation.isPending,
    ],
  );

  const table = useReactTable({
    data: usersList ?? EMPTY_DATA,
    columns: columnDefs,

    globalFilterFn,

    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  return (
    <>
      <Card className="h-full border border-border-button bg-transparent rounded-xl overflow-x-hidden overflow-y-auto">
        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.userManagement')}</CardTitle>

            <div className="ml-auto flex justify-end gap-4">
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
                    <EnterpriseFeature>
                      {() => (
                        <section>
                          <div className="font-bold mb-3">
                            {t('admin.role')}
                          </div>

                          <RadioGroup
                            value={
                              (table
                                .getColumn('role')
                                ?.getFilterValue() as string) ?? ''
                            }
                            onValueChange={(value) =>
                              table.getColumn('role')?.setFilterValue(value)
                            }
                          >
                            <Label className="space-x-2">
                              <RadioGroupItem value="" />
                              <span>{t('admin.all')}</span>
                            </Label>

                            {roleList?.map(({ id, role_name }) => (
                              <Label key={id} className="space-x-2">
                                <RadioGroupItem
                                  className="bg-bg-input border-border-button"
                                  value={role_name}
                                />
                                <span>{role_name}</span>
                              </Label>
                            ))}
                          </RadioGroup>
                        </section>
                      )}
                    </EnterpriseFeature>

                    <section>
                      <div className="font-bold mb-3">{t('admin.status')}</div>

                      <RadioGroup
                        value={
                          (table
                            .getColumn('is_active')
                            ?.getFilterValue() as string) ?? ''
                        }
                        onValueChange={(value) =>
                          table.getColumn('is_active')?.setFilterValue(value)
                        }
                      >
                        {STATUS_FILTER_OPTIONS.map(({ label, value }) => (
                          <Label key={value} className="space-x-2">
                            <RadioGroupItem
                              className="bg-bg-input border-border-button"
                              value={value}
                            />
                            <span>{t(label)}</span>
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

              <Button
                className="h-10 px-4"
                onClick={() => setCreateUserModalOpen(true)}
              >
                <LucideUserPlus />
                {t('admin.newUser')}
              </Button>
            </div>
          </CardHeader>

          <CardContent>
            <Table>
              <colgroup>
                <col width="*" />
                <col className="w-[22%]" />

                <EnterpriseFeature>
                  {() => <col className="w-[12%]" />}
                </EnterpriseFeature>

                <col className="w-[10%]" />
                <col className="w-[12%]" />
                <col className="w-52" />
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
                  <TableEmpty key="empty" columnsLength={columnDefs.length} />
                )}
              </TableBody>
            </Table>
          </CardContent>

          <CardFooter className="flex items-center justify-end">
            <RAGFlowPagination
              total={usersList?.length ?? 0}
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
        <DialogContent className="p-0 border-border-button">
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.deleteUser')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4">
            <DialogDescription className="text-text-primary">
              {t('admin.deleteUserConfirmation')}

              <div className="rounded-lg mt-6 p-4 border border-border-button">
                {userToMakeAction?.email}
              </div>
            </DialogDescription>
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={deleteUserMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              className="px-4 h-10"
              variant="destructive"
              onClick={() =>
                userToMakeAction &&
                deleteUserMutation.mutate(userToMakeAction?.email)
              }
              disabled={deleteUserMutation.isPending}
              loading={deleteUserMutation.isPending}
            >
              {t('admin.delete')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Change Password Modal */}
      <Dialog open={passwordModalOpen} onOpenChange={setPasswordModalOpen}>
        <DialogContent
          className="p-0 border-border-button"
          onAnimationEnd={() => {
            if (!passwordModalOpen) {
              changePasswordForm.form.reset();
            }
          }}
        >
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.changePassword')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4 text-text-secondary">
            <changePasswordForm.FormComponent
              key="changePasswordForm"
              email={userToMakeAction?.email || ''}
              onSubmit={({ newPassword }) => {
                if (userToMakeAction) {
                  changePasswordMutation.mutate({
                    email: userToMakeAction.email,
                    password: newPassword,
                  });
                }
              }}
            />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setPasswordModalOpen(false);
                setUserToMakeAction(null);
              }}
              disabled={changePasswordMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              form={changePasswordForm.id}
              className="px-4 h-10"
              variant="default"
              type="submit"
              disabled={changePasswordMutation.isPending}
              loading={changePasswordMutation.isPending}
            >
              {t('admin.changePassword')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Create User Modal */}
      <Dialog
        open={createUserModalOpen}
        onOpenChange={() => {
          setCreateUserModalOpen(false);
          createUserForm.form.reset();
        }}
      >
        <DialogContent className="p-0 border-border-button">
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.createNewUser')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4">
            <createUserForm.FormComponent
              id={createUserForm.id}
              onSubmit={createUserMutation.mutate}
            />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => {
                setCreateUserModalOpen(false);
                createUserForm.form.reset();
              }}
              disabled={createUserMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              form={createUserForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={createUserMutation.isPending}
              loading={createUserMutation.isPending}
            >
              {t('admin.confirm')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminUserManagement;
