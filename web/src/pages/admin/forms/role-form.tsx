import { useCallback, useId, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery } from '@tanstack/react-query';
import { z } from 'zod';

import { Card, CardContent } from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { listResources } from '@/services/admin-service';
import { PERMISSION_TYPES, formMergeDefaultValues } from '../utils';

export interface CreateRoleFormData {
  name: string;
  description: string;
  permissions: Record<string, AdminService.PermissionData>;
}

interface CreateRoleFormProps {
  id: string;
  form: ReturnType<typeof useForm<CreateRoleFormData>>;
  onSubmit?: (data: CreateRoleFormData) => void;
}

export const CreateRoleForm = ({
  id,
  form,
  onSubmit = () => {},
}: CreateRoleFormProps) => {
  const { t } = useTranslation();

  const { data: resourceTypes } = useQuery({
    queryKey: ['admin/resourceTypes'],
    queryFn: async () => (await listResources()).data.data.resource_types,
    retry: false,
  });

  return (
    <Form {...form}>
      <form
        id={id}
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {/* Role name field */}
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium" required>
                {t('admin.roleName')}
              </FormLabel>
              <FormControl>
                <Input
                  placeholder={t('admin.roleName')}
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Role description field */}
        <FormField
          control={form.control}
          name="description"
          render={({ field }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.description')}
              </FormLabel>
              <FormControl>
                <Input
                  placeholder={t('admin.description')}
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button"
                  {...field}
                />
              </FormControl>
            </FormItem>
          )}
        />

        {/* Permissions section */}
        <div>
          <Label>{t('admin.resources')}</Label>

          <Tabs defaultValue={resourceTypes?.[0]} className="w-full mt-2">
            <TabsList className="p-0 mb-2 gap-4 bg-transparent justify-start">
              {resourceTypes?.map((resourceType) => (
                <TabsTrigger
                  key={resourceType}
                  value={resourceType}
                  className="text-text-secondary border-0.5 border-border-button data-[state=active]:bg-bg-card"
                >
                  {t(`admin.resourceType.${resourceType.toLowerCase()}`)}
                </TabsTrigger>
              ))}
            </TabsList>

            {resourceTypes?.map((resourceType) => (
              <TabsContent
                key={resourceType}
                value={resourceType}
                className="space-y-4"
              >
                <Card className="border-0 bg-bg-card !shadow-none">
                  <CardContent className="p-6">
                    <div className="grid grid-cols-4 gap-4">
                      {PERMISSION_TYPES.map((permissionType) => (
                        <FormField
                          key={permissionType}
                          name={`permissions.${resourceType}.${permissionType}`}
                          render={({ field }) => (
                            <FormItem className="space-y-0 inline-flex items-center gap-2">
                              <FormLabel>
                                {t(`admin.permissionType.${permissionType}`)}
                              </FormLabel>
                              <FormControl>
                                <Switch
                                  {...field}
                                  checked={field.value}
                                  onCheckedChange={field.onChange}
                                />
                              </FormControl>
                            </FormItem>
                          )}
                        />
                      ))}
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
            ))}
          </Tabs>
        </div>
      </form>
    </Form>
  );
};

// Export the form validation state for parent component
function useCreateRoleForm(props?: {
  defaultValues:
    | Partial<CreateRoleFormData>
    | (() => Promise<CreateRoleFormData>);
}) {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z.object({
      name: z.string().min(1, { message: t('admin.roleNameRequired') }),
      description: z.string().optional(),
      permissions: z.record(
        z.string(),
        z.object({
          enable: z.boolean().optional(),
          read: z.boolean().optional(),
          write: z.boolean().optional(),
          share: z.boolean().optional(),
        }),
      ),
    });
  }, [t]);

  const form = useForm<CreateRoleFormData>({
    defaultValues: formMergeDefaultValues(
      {
        name: '',
        description: '',
        permissions: {},
      },
      props?.defaultValues,
    ),
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<CreateRoleFormProps>) => (
      <CreateRoleForm id={id} form={form} {...props} />
    ),
    [id, form],
  );

  return {
    schema,
    id,
    form,
    FormComponent,
  };
}

export default useCreateRoleForm;
