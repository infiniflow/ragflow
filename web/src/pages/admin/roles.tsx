import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useQuery } from '@tanstack/react-query';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Label } from '@/components/ui/label';
import { LoadingButton } from '@/components/ui/loading-button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Switch } from '@/components/ui/switch';
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs-underlined';
import { LucideEdit3, LucideTrash2, LucideUserPlus } from 'lucide-react';

import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { listRolesWithPermission } from '@/services/admin-service';

import useCreateRoleForm from './forms/role-form';

// #region FAKE DATA
function _pickRandom<T extends unknown>(arr: T[]): T | void {
  return arr[Math.floor(Math.random() * arr.length)];
}

const PSEUDO_TABLE_ITEMS = Array.from({ length: 20 }, () => ({
  id: Math.random().toString(36).slice(2, 8),
  name: 'Ahaha',
  description: 'Ahaha description',
  permissions: {
    dataset: {
      enable: _pickRandom([true, false]),
      read: _pickRandom([true, false]),
      write: _pickRandom([true, false]),
      share: _pickRandom([true, false]),
    },
    agent: {
      enable: _pickRandom([true, false]),
      read: _pickRandom([true, false]),
      write: _pickRandom([true, false]),
      share: _pickRandom([true, false]),
    },
  },
}));
// #endregion

function AdminRoles() {
  const { t } = useTranslation();
  const [isAddRoleModalOpen, setIsAddRoleModalOpen] = useState(false);

  const { data: roleList } = useQuery({
    queryKey: ['admin/listRolesWithPermission'],
    queryFn: async () => (await listRolesWithPermission()).data.data.roles,
  });

  const createRoleForm = useCreateRoleForm();

  const handleAddRole = (data: any) => {
    console.log('New role data:', data);
    // TODO: Implement actual role creation logic
    createRoleForm.form.reset();
    setIsAddRoleModalOpen(false);
  };

  return (
    <>
      <Card className="h-full border border-border-button bg-transparent rounded-xl">
        <ScrollArea className="size-full">
          <CardHeader className="space-y-0 flex flex-row justify-between items-center">
            <CardTitle>{t('admin.roles')}</CardTitle>

            <Button
              className="h-10 px-4"
              onClick={() => setIsAddRoleModalOpen(true)}
            >
              <LucideUserPlus />
              {t('admin.newRole')}
            </Button>
          </CardHeader>

          <CardContent className="space-y-6">
            {roleList?.map((role) => {
              const resources = Object.entries(role.permissions);

              return (
                <Card
                  key={role.id}
                  className="group border border-border-default bg-transparent hover:bg-bg-card transition-color duration-150"
                >
                  <CardHeader className="space-y-0 flex flex-row items-center border-b border-border-button">
                    <div className="space-y-1.5">
                      <CardTitle className="font-normal text-xl">
                        {role.role_name}
                      </CardTitle>
                      <div className="text-sm text-text-secondary">
                        {role.description || (
                          <i className="text-muted-foreground">
                            {t('admin.noDescription')}
                          </i>
                        )}

                        <Button
                          variant="transparent"
                          className="ml-2 p-0 border-0 size-[1em] align-middle opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                        >
                          <LucideEdit3 className="!size-[1em]" />
                        </Button>
                      </div>
                    </div>

                    <Button
                      variant="ghost"
                      size="icon"
                      className="ml-auto opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                    >
                      <LucideTrash2 />
                    </Button>
                  </CardHeader>

                  <CardContent className="p-6">
                    <Tabs
                      className="h-full flex flex-col"
                      defaultValue={resources[0]?.[0]}
                    >
                      <TabsList className="p-0 mb-2 gap-4 bg-transparent">
                        {resources.map(([name]) => (
                          <TabsTrigger
                            key={name}
                            value={name}
                            className="border-border-button dark:data-[state=active]:bg-bg-input"
                          >
                            {t(`admin.resourceType.${name}`)}
                          </TabsTrigger>
                        ))}
                      </TabsList>

                      {resources.map(([name, permission]) => (
                        <TabsContent key={name} value={name}>
                          <div className="flex gap-8">
                            <Label className="flex items-center gap-2">
                              <Switch
                                checked={!!permission.enable}
                                onCheckedChange={console.log}
                              />
                              {t('admin.enable')}
                            </Label>

                            <Label className="flex items-center gap-2">
                              <Switch
                                checked={!!permission.read}
                                onCheckedChange={() => {}}
                              />
                              {t('admin.read')}
                            </Label>

                            <Label className="flex items-center gap-2">
                              <Switch
                                checked={!!permission.write}
                                onCheckedChange={() => {}}
                              />
                              {t('admin.write')}
                            </Label>

                            <Label className="flex items-center gap-2">
                              <Switch
                                checked={!!permission.share}
                                onCheckedChange={() => {}}
                              />
                              {t('admin.share')}
                            </Label>
                          </div>
                        </TabsContent>
                      ))}
                    </Tabs>
                  </CardContent>
                </Card>
              );
            })}
          </CardContent>
        </ScrollArea>
      </Card>

      {/* Add Role Modal */}
      <Dialog open={isAddRoleModalOpen} onOpenChange={setIsAddRoleModalOpen}>
        <DialogContent className="max-w-2xl p-0 border-border-button">
          <DialogHeader className="p-6 border-b border-border-button">
            <DialogTitle>{t('admin.addNewRole')}</DialogTitle>
          </DialogHeader>

          <section className="px-12 py-4">
            <createRoleForm.FormComponent onSubmit={handleAddRole} />
          </section>

          <DialogFooter className="flex justify-end gap-4 px-12 pt-4 pb-8">
            <Button
              className="px-4 h-10 dark:border-border-button"
              variant="outline"
              onClick={() => setIsAddRoleModalOpen(false)}
            >
              {t('admin.cancel')}
            </Button>

            <LoadingButton
              type="submit"
              form={createRoleForm.id}
              className="px-4 h-10"
              variant="default"
            >
              {t('admin.confirm')}
            </LoadingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default AdminRoles;
