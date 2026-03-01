import { useContext, useLayoutEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';

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
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import {
  LucideClipboardList,
  LucideDot,
  LucideTrash2,
  LucideUserLock,
  LucideUserPlus,
} from 'lucide-react';

import { rsaPsw } from '@/utils';

import Spotlight from '@/components/spotlight';
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
import { Routes } from '@/routes';
import { LucideFilter, LucideSearch } from 'lucide-react';

import useChangePasswordForm from './forms/change-password-form';
import useCreateUserForm from './forms/user-form';

import {
  createUser,
  deleteUser,
  grantSuperuser,
  listRoles,
  listUsers,
  revokeSuperuser,
  updateUserPassword,
  updateUserRole,
  updateUserStatus,
} from '@/services/admin-service';

import {
  createColumnFilterFn,
  createFuzzySearchFn,
  EMPTY_DATA,
  IS_ENTERPRISE,
  parseBooleanish,
} from './utils';

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import EnterpriseFeature from './components/enterprise-feature';
import { CurrentUserInfoContext } from './layouts/root-layout';

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
  const [{ userInfo }] = useContext(CurrentUserInfoContext);

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
    placeholderData: keepPreviousData,
  });

  const { data: usersList } = useQuery({
    queryKey: ['admin/listUsers'],
    queryFn: async () => (await listUsers()).data.data,
    retry: false,
    placeholderData: keepPreviousData,
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

  const setSuperuserMutation = useMutation({
    mutationFn: ({
      email,
      type,
    }: {
      email: string;
      type: 'grant' | 'revoke';
    }) => {
      return type === 'grant' ? grantSuperuser(email) : revokeSuperuser(email);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin/listUsers'] });
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
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" className="min-w-16">
                      {cell.getValue()}
                    </Button>
                  </DropdownMenuTrigger>

                  <DropdownMenuContent>
                    {roleList?.map(({ id, role_name }) => (
                      <DropdownMenuItem
                        key={id}
                        onClick={() => {
                          updateUserRoleMutation.mutate({
                            email: row.original.email,
                            role: role_name,
                          });
                        }}
                      >
                        {role_name}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>
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

      columnHelper.accessor('is_active', {
        header: t('admin.status'),
        cell: ({ cell, row }) => {
          const isMe = row.original.email === userInfo?.email;

          if (isMe) {
            return (
              <Badge
                variant={
                  parseBooleanish(cell.getValue()) ? 'success' : 'destructive'
                }
              >
                <LucideDot className="size-[1em] stroke-[8] mr-1" />
                {parseBooleanish(cell.getValue())
                  ? t('admin.active')
                  : t('admin.inactive')}
              </Badge>
            );
          }

          return (
            <Select
              disabled={updateUserStatusMutation.isPending}
              value={cell.getValue()}
              onValueChange={(value) =>
                updateUserStatusMutation.mutate({
                  email: row.original.email,
                  isActive: parseBooleanish(value),
                })
              }
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>

              <SelectContent>
                <SelectItem value="0">
                  <div className="flex items-center">
                    <LucideDot className="size-[1em] stroke-[8] mr-1" />
                    {t('admin.inactive')}
                  </div>
                </SelectItem>

                <SelectItem value="1">
                  <div className="flex items-center text-state-success">
                    <LucideDot className="size-[1em] stroke-[8] mr-1" />
                    {t('admin.active')}
                  </div>
                </SelectItem>
              </SelectContent>
            </Select>
          );
        },
        filterFn: createColumnFilterFn(
          (row, id, filterValue) => row.getValue(id) === filterValue,
          {
            autoRemove: (v) => !v,
            resolveFilterValue: (v) =>
              v ? (v === 'active' ? '1' : '0') : null,
          },
        ),
      }),

      columnHelper.accessor('is_superuser', {
        header: t('admin.userType'),
        cell: ({ cell, row }) => {
          const isMe = row.original.email === userInfo?.email;

          if (isMe) {
            return <Badge variant="secondary">{t('admin.superuser')}</Badge>;
          }

          return (
            <Select
              disabled={
                setSuperuserMutation.isPending ||
                row.original.email === userInfo?.email
              }
              value={cell.getValue() ? 'superuser' : 'normal'}
              onValueChange={(value) => {
                setSuperuserMutation.mutate({
                  email: row.original.email,
                  type: value === 'superuser' ? 'grant' : 'revoke',
                });
              }}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>

              <SelectContent>
                <SelectItem value="normal">{t('admin.normalUser')}</SelectItem>
                <SelectItem value="superuser">
                  {t('admin.superuser')}
                </SelectItem>
              </SelectContent>
            </Select>
          );
        },
      }),

      columnHelper.display({
        id: 'actions',
        header: t('admin.actions'),
        cell: ({ row }) => {
          const isMe = row.original.email === userInfo?.email;

          return (
            <div className="opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100 transition-opacity">
              <Button
                variant="transparent"
                size="icon"
                className="border-0"
                onClick={() =>
                  navigate(
                    `${Routes.AdminUserManagement}/${row.original.email}`,
                  )
                }
              >
                <LucideClipboardList />
              </Button>

              {!isMe && (
                <>
                  <Button
                    variant="transparent"
                    size="icon"
                    className="border-0"
                    onClick={() => {
                      setUserToMakeAction(row.original);
                      setPasswordModalOpen(true);
                    }}
                  >
                    <LucideUserLock />
                  </Button>
                  <Button
                    variant="danger"
                    size="icon"
                    className="border-0"
                    onClick={() => {
                      setUserToMakeAction(row.original);
                      setDeleteModalOpen(true);
                    }}
                  >
                    <LucideTrash2 />
                  </Button>
                </>
              )}
            </div>
          );
        },
      }),
    ],
    [
      t,
      roleList,
      updateUserRoleMutation,
      userInfo?.email,
      updateUserStatusMutation,
      setSuperuserMutation,
      navigate,
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

    autoResetPageIndex: false,
  });

  useLayoutEffect(() => {
    if (table.getState().pagination.pageIndex > table.getPageCount()) {
      table.setPageIndex(Math.max(0, table.getPageCount() - 1));
    }
  }, [usersList, table]);

  return (
    <>
      <Card className="!shadow-none relative h-full bg-transparent overflow-hidden">
        <Spotlight />

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
                            <Label className="flex items-center space-x-2">
                              <RadioGroupItem value="" />
                              <span>{t('admin.all')}</span>
                            </Label>

                            {roleList?.map(({ id, role_name }) => (
                              <Label
                                key={id}
                                className="flex items-center space-x-2"
                              >
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
                          <Label
                            key={value}
                            className="flex items-center space-x-2"
                          >
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
                  {() => <col className="w-24" />}
                </EnterpriseFeature>

                <col className="w-40" />
                <col className="w-40" />
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
                    <TableRow key={row.id} className="group/row">
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('admin.deleteUser')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <DialogDescription>
              {t('admin.deleteUserConfirmation')}
            </DialogDescription>

            <div className="rounded-lg mt-6 p-4 border-0.5 border-border-button">
              {userToMakeAction?.email}
            </div>
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={deleteUserMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
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
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Change Password Modal */}
      <Dialog open={passwordModalOpen} onOpenChange={setPasswordModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!passwordModalOpen) {
              changePasswordForm.form.reset();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.changePassword')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
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

          <DialogFooter className="gap-4 px-6 py-4">
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

            <Button
              form={changePasswordForm.id}
              className="px-4 h-10"
              variant="default"
              type="submit"
              disabled={changePasswordMutation.isPending}
              loading={changePasswordMutation.isPending}
            >
              {t('admin.changePassword')}
            </Button>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('admin.createNewUser')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <createUserForm.FormComponent
              id={createUserForm.id}
              onSubmit={createUserMutation.mutate}
            />
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
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

            <Button
              form={createUserForm.id}
              type="submit"
              className="px-4 h-10"
              variant="default"
              disabled={createUserMutation.isPending}
              loading={createUserMutation.isPending}
            >
              {t('admin.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminUserManagement;
