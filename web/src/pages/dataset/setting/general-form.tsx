import { AvatarUpload } from '@/components/avatar-upload';
import { FormContainer } from '@/components/form-container';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button, ButtonLoading } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { PermissionRole } from '@/constants/permission';
import { useUpdateKnowledge } from '@/hooks/use-knowledge-request';
import { useMemo } from 'react';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';

export function GeneralForm() {
  const form = useFormContext();
  const { t } = useTranslation();

  const { saveKnowledgeConfiguration, loading: submitLoading } =
    useUpdateKnowledge();

  const defaultValues = useMemo(
    () => form.formState.defaultValues ?? {},
    [form.formState.defaultValues],
  );
  const parser_id = defaultValues['parser_id'];
  const { id: kb_id } = useParams();

  const teamOptions = useMemo(() => {
    return Object.values(PermissionRole).map((x) => ({
      label: t('knowledgeConfiguration.' + x),
      value: x,
    }));
  }, [t]);

  return (
    <>
      <FormContainer className="space-y-10 p-10">
        <FormField
          control={form.control}
          name="name"
          render={({ field }) => (
            <FormItem className="items-center space-y-0">
              <div className="flex">
                <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                  <span className="text-red-600">*</span>
                  {t('common.name')}
                </FormLabel>
                <FormControl className="w-3/4">
                  <Input {...field}></Input>
                </FormControl>
              </div>
              <div className="flex pt-1">
                <div className="w-1/4"></div>
                <FormMessage />
              </div>
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="avatar"
          render={({ field }) => (
            <FormItem className="items-center space-y-0">
              <div className="flex">
                <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                  {t('setting.avatar')}
                </FormLabel>
                <FormControl className="w-3/4">
                  <AvatarUpload {...field}></AvatarUpload>
                </FormControl>
              </div>
              <div className="flex pt-1">
                <div className="w-1/4"></div>
                <FormMessage />
              </div>
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="description"
          render={({ field }) => {
            // null initialize empty string
            if (typeof field.value === 'object' && !field.value) {
              form.setValue('description', '  ');
            }
            return (
              <FormItem className="items-center space-y-0">
                <div className="flex">
                  <FormLabel className="text-sm text-muted-foreground whitespace-nowrap w-1/4">
                    {t('flow.description')}
                  </FormLabel>
                  <FormControl className="w-3/4">
                    <Input {...field}></Input>
                  </FormControl>
                </div>
                <div className="flex pt-1">
                  <div className="w-1/4"></div>
                  <FormMessage />
                </div>
              </FormItem>
            );
          }}
        />
        <RAGFlowFormItem
          name="permission"
          label={t('knowledgeConfiguration.permissions')}
          tooltip={t('knowledgeConfiguration.permissionsTip')}
          horizontal
        >
          <SelectWithSearch
            options={teamOptions}
            triggerClassName="w-3/4"
          ></SelectWithSearch>
        </RAGFlowFormItem>
      </FormContainer>
      <div className="text-right pt-4 flex justify-end gap-3">
        <Button
          type="reset"
          variant={'outline'}
          onClick={() => {
            form.reset();
          }}
        >
          {t('knowledgeConfiguration.cancel')}
        </Button>
        <ButtonLoading
          type="button"
          loading={submitLoading}
          onClick={() => {
            (async () => {
              let isValidate = await form.trigger('name');
              const { name, description, permission, avatar } =
                form.getValues();

              if (isValidate) {
                saveKnowledgeConfiguration({
                  kb_id,
                  parser_id,
                  name,
                  description,
                  avatar,
                  permission,
                });
              }
            })();
          }}
        >
          {t('knowledgeConfiguration.save')}
        </ButtonLoading>
      </div>
    </>
  );
}
