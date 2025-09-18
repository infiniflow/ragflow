import { AvatarUpload } from '@/components/avatar-upload';
import { FormContainer } from '@/components/form-container';
import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { PermissionFormField } from './permission-form-field';
import { GeneralSavingButton } from './saving-button';

export function GeneralForm() {
  const form = useFormContext();
  const { t } = useTranslation();

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
        <PermissionFormField></PermissionFormField>
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
        <GeneralSavingButton></GeneralSavingButton>
      </div>
    </>
  );
}
