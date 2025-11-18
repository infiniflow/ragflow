import { AvatarUpload } from '@/components/avatar-upload';
import PageRankFormField from '@/components/page-rank-form-field';
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
import { TagItems } from './components/tag-item';
import { EmbeddingModelItem } from './configuration/common-item';
import { PermissionFormField } from './permission-form-field';

export function GeneralForm() {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <>
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem className="items-center space-y-0">
            <div className="flex">
              <FormLabel className="text-sm whitespace-nowrap w-1/4">
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
              <FormLabel className="text-sm  whitespace-nowrap w-1/4">
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
                <FormLabel className="text-sm  whitespace-nowrap w-1/4">
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
      <EmbeddingModelItem isEdit={true}></EmbeddingModelItem>
      <PageRankFormField></PageRankFormField>

      <TagItems></TagItems>
    </>
  );
}
