import { mapKeys } from 'lodash';

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
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
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { LucideEdit3, LucideTrash2, LucideUserPlus } from 'lucide-react';

import {
  assignRolePermissions,
  createRole,
  deleteRole,
  listResources,
  listRolesWithPermission,
  revokeRolePermissions,
  updateRoleDescription,
} from '@/services/admin-service';

import Empty from '@/components/empty/empty';
import useCreateRoleForm, { CreateRoleFormData } from './forms/role-form';
import { PERMISSION_TYPES } from './utils';

function AdminRoles() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const createRoleForm = useCreateRoleForm();

  const [isAddRoleModalOpen, setAddRoleModalOpen] = useState(false);

  const editRoleDescriptionFormId = useId();
  const [isEditRoleDescriptionModalOpen, setEditRoleDescriptionModalOpen] =
    useState(false);
  const [roleDescription, setRoleDescription] = useState('');

  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [roleToMakeAction, setRoleToMakeAction] =
    useState<AdminService.ListRoleItemWithPermission | null>(null);

  const { data: roleList } = useQuery({
    queryKey: ['admin/listRolesWithPermission'],
    queryFn: async () => (await listRolesWithPermission())?.data?.data?.roles,
    retry: false,
  });

  const { data: resourceTypes } = useQuery({
    queryKey: ['admin/resourceTypes'],
    queryFn: async () => (await listResources()).data.data.resource_types,
    retry: false,
  });

  const updateRoleDescriptionMutation = useMutation({
    mutationFn: (data: { name: string; description: string }) =>
      updateRoleDescription(data.name, data.description),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['admin/listRolesWithPermission'],
      });
      setEditRoleDescriptionModalOpen(false);
      setRoleToMakeAction(null);
    },
    retry: false,
  });

  const updateRolePermissionsMutation = useMutation({
    mutationFn: (data: {
      name: string;
      resourceName: string;
      permissionType: (typeof PERMISSION_TYPES)[number];
      value: boolean;
    }) => {
      const permissionDiffData = {
        [data.resourceName.toLowerCase()]: {
          [data.permissionType]: data.value,
        },
      };

      return data.value
        ? assignRolePermissions(data.name, permissionDiffData)
        : revokeRolePermissions(data.name, permissionDiffData);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['admin/listRolesWithPermission'],
      });
    },
    retry: false,
  });

  const createRoleMutation = useMutation({
    mutationFn: async (data: CreateRoleFormData) => {
      const { data: { data: createdRoleDetail } = {} } = await createRole({
        roleName: data.name,
        description: data.description,
      });

      if (!createdRoleDetail) {
        throw new Error();
      }

      await assignRolePermissions(
        data.name,
        mapKeys(data.permissions, (_, key) => key.toLowerCase()),
      );
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['admin/listRolesWithPermission'],
      });
      createRoleForm.form.reset();
      setAddRoleModalOpen(false);
    },
    retry: false,
  });

  const deleteRoleMutation = useMutation({
    mutationFn: (roleName: string) => deleteRole(roleName),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ['admin/listRolesWithPermission'],
      });
      setDeleteModalOpen(false);
      setRoleToMakeAction(null);
    },
    retry: false,
  });

  return (
    <>
      <Card className="!shadow-none relative w-full h-full border-0.5 border-border-button bg-transparent rounded-xl">
        <Spotlight />

        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.roles')}</CardTitle>

            <Button
              className="h-10 px-4"
              onClick={() => setAddRoleModalOpen(true)}
            >
              <LucideUserPlus />
              {t('admin.newRole')}
            </Button>
          </CardHeader>

          <CardContent className="space-y-6">
            {roleList?.length ? (
              roleList.map((role) => (
                <Card
                  key={role.id}
                  className="group/role border-0.5 border-border-default bg-transparent dark:hover:bg-bg-card transition-color duration-150"
                >
                  <CardHeader className="space-y-0 flex flex-row gap-4 items-center border-b-0.5 border-border-button">
                    <div className="space-y-1.5 w-0 flex-1">
                      <CardTitle className="font-normal text-xl">
                        {role.role_name}
                      </CardTitle>

                      <div className="text-sm text-text-secondary break-words">
                        {role.description || (
                          <i className="text-muted-foreground">
                            {t('admin.noDescription')}
                          </i>
                        )}

                        <Button
                          variant="transparent"
                          className="
                            ml-2 p-0 border-0 size-[1em] align-middle opacity-0
                            group-hover/role:opacity-100 group-focus-within/role:opacity-100
                          "
                          onClick={() => {
                            setEditRoleDescriptionModalOpen(true);
                            setRoleToMakeAction(role);
                            setRoleDescription(role.description || '');
                          }}
                        >
                          <LucideEdit3 className="!size-[1em]" />
                        </Button>
                      </div>
                    </div>

                    <Button
                      variant="ghost"
                      size="icon"
                      className="ml-auto opacity-0 group-hover/role:opacity-100 group-focus-within/role:opacity-100"
                      disabled={deleteRoleMutation.isPending}
                      onClick={() => {
                        setDeleteModalOpen(true);
                        setRoleToMakeAction(role);
                      }}
                    >
                      <LucideTrash2 />
                    </Button>
                  </CardHeader>

                  <CardContent className="p-6">
                    <Tabs
                      className="h-full flex flex-col"
                      defaultValue={resourceTypes?.[0]}
                    >
                      <TabsList className="p-0 mb-2 gap-4 bg-transparent justify-start">
                        {resourceTypes?.map((resourceName) => (
                          <TabsTrigger
                            key={resourceName}
                            value={resourceName}
                            className="text-text-secondary border-0.5 border-border-button data-[state=active]:bg-bg-card"
                          >
                            {t(
                              `admin.resourceType.${resourceName.toLowerCase()}`,
                            )}
                          </TabsTrigger>
                        ))}
                      </TabsList>

                      {resourceTypes?.map((resourceName) => {
                        const permission =
                          role.permissions[resourceName.toLowerCase()];

                        return (
                          <TabsContent key={resourceName} value={resourceName}>
                            <Card className="border-0 bg-bg-card !shadow-none">
                              <CardContent className="p-6 flex gap-8">
                                {PERMISSION_TYPES.map((permissionType) => (
                                  <Label
                                    key={permissionType}
                                    className="flex items-center gap-2"
                                  >
                                    {t(`admin.${permissionType}`)}

                                    <Switch
                                      disabled={
                                        updateRolePermissionsMutation.isPending
                                      }
                                      checked={!!permission?.[permissionType]}
                                      onCheckedChange={(value) =>
                                        updateRolePermissionsMutation.mutate({
                                          name: role.role_name,
                                          resourceName:
                                            resourceName.toLowerCase(),
                                          permissionType,
                                          value,
                                        })
                                      }
                                    />
                                  </Label>
                                ))}
                              </CardContent>
                            </Card>
                          </TabsContent>
                        );
                      })}
                    </Tabs>
                  </CardContent>
                </Card>
              ))
            ) : (
              <Empty className="py-24" />
            )}
          </CardContent>
        </ScrollArea>
      </Card>

      {/* Add role modal */}
      <Dialog open={isAddRoleModalOpen} onOpenChange={setAddRoleModalOpen}>
        <DialogContent
          className="max-w-2xl"
          onAnimationEnd={() => {
            if (!isAddRoleModalOpen) {
              createRoleForm.form.reset();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.addNewRole')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <createRoleForm.FormComponent
              onSubmit={createRoleMutation.mutate}
            />
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setAddRoleModalOpen(false)}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              type="submit"
              form={createRoleForm.id}
              className="px-4 h-10"
              variant="default"
            >
              {t('admin.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Modify role description modal */}
      <Dialog
        open={isEditRoleDescriptionModalOpen}
        onOpenChange={setEditRoleDescriptionModalOpen}
      >
        <DialogContent
          onAnimationEnd={() => {
            if (!isEditRoleDescriptionModalOpen) {
              setRoleToMakeAction(null);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.editRoleDescription')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <form
              id={editRoleDescriptionFormId}
              onSubmit={(evt) => {
                evt.preventDefault();
                updateRoleDescriptionMutation.mutate({
                  name: roleToMakeAction!?.role_name,
                  description: roleDescription.trim(),
                });
              }}
            >
              <Label>
                <div className="text-sm font-medium">
                  {t('admin.description')}
                </div>

                <Input
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  value={roleDescription}
                  onInput={(evt) => setRoleDescription(evt.currentTarget.value)}
                />
              </Label>
            </form>
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setEditRoleDescriptionModalOpen(false)}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              type="submit"
              form={editRoleDescriptionFormId}
              className="px-4 h-10"
            >
              {t('admin.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete role modal */}
      <Dialog open={deleteModalOpen} onOpenChange={setDeleteModalOpen}>
        <DialogContent
          onAnimationEnd={() => {
            if (!deleteModalOpen) {
              setRoleToMakeAction(null);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>{t('admin.deleteRole')}</DialogTitle>
          </DialogHeader>

          <section className="px-6">
            <DialogDescription className="text-text-primary">
              {t('admin.deleteRoleConfirmation')}
            </DialogDescription>

            <div className="rounded-lg mt-6 p-4 border-0.5 border-border-button">
              {roleToMakeAction?.role_name}
            </div>
          </section>

          <DialogFooter className="gap-4 px-6 py-4">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setDeleteModalOpen(false)}
              disabled={deleteRoleMutation.isPending}
            >
              {t('admin.cancel')}
            </Button>

            <Button
              className="px-4 h-10"
              variant="destructive"
              onClick={() =>
                roleToMakeAction &&
                deleteRoleMutation.mutate(roleToMakeAction!?.role_name)
              }
              disabled={deleteRoleMutation.isPending}
              loading={deleteRoleMutation.isPending}
            >
              {t('admin.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminRoles;
