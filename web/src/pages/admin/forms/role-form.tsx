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
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/tabs-underlined';
import { AdminService, listResources } from '@/services/admin-service';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useId, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

interface CreateRoleFormData {
  name: string;
  description: string;
  permissions: Record<string, AdminService.PermissionData>;
}

interface CreateRoleFormProps {
  id: string;
  form: ReturnType<typeof useForm<CreateRoleFormData>>;
  onSubmit?: (data: CreateRoleFormData) => void;
}

const PERMISSION_TYPES = ['enable', 'read', 'write', 'share'] as const;

export const CreateRoleForm = ({
  id,
  form,
  onSubmit = () => {},
}: CreateRoleFormProps) => {
  const { t } = useTranslation();

  const { data: resourceTypes } = useQuery({
    queryKey: ['admin/resourceTypes'],
    queryFn: async () => (await listResources()).data.data.resource_types,
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
            <TabsList className="p-0 mb-2 gap-4 bg-transparent">
              {resourceTypes?.map((resourceType) => (
                <TabsTrigger
                  key={resourceType}
                  value={resourceType}
                  className="text-text-secondary border-border-button dark:data-[state=active]:bg-bg-input"
                >
                  {t(`admin.resourceType.${resourceType}`)}
                </TabsTrigger>
              ))}
            </TabsList>

            {resourceTypes?.map((resourceType) => (
              <TabsContent
                key={resourceType}
                value={resourceType}
                className="space-y-4"
              >
                <Card className="border-0 bg-bg-card">
                  <CardContent className="p-6">
                    <div className="grid grid-cols-4 gap-4">
                      {PERMISSION_TYPES.map((permissionType) => (
                        <FormField
                          key={permissionType}
                          name={`permissions.${resourceType}.${permissionType}`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel className="flex items-center gap-2">
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
  defaultValues: Partial<CreateRoleFormData>;
}) {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z.object({
      name: z.string().min(1, { message: 'Role name is required' }),
      description: z.string().optional(),
      permissions: z.record(
        z.string(),
        z.object({
          enable: z.boolean(),
          read: z.boolean(),
          write: z.boolean(),
          share: z.boolean(),
        }),
      ),
    });
  }, [t]);

  const form = useForm<CreateRoleFormData>({
    defaultValues: {
      name: '',
      description: '',
      permissions: {},
      ...(props?.defaultValues ?? {}),
    },
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<CreateRoleFormProps>) => (
      <CreateRoleForm id="create-role-form" form={form} {...props} />
    ),
    [form],
  );

  return {
    schema,
    id,
    form,
    FormComponent,
  };
}

export default useCreateRoleForm;
