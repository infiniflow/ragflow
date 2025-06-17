import { FileUploader } from '@/components/file-uploader';
import { FormContainer } from '@/components/form-container';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function GeneralForm() {
  const form = useFormContext();
  const { t } = useTranslation();

  return (
    <FormContainer className="space-y-2 p-10">
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('knowledgeConfiguration.name')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="description"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('knowledgeConfiguration.description')}</FormLabel>
            <FormControl>
              <Input {...field}></Input>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="avatar"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('knowledgeConfiguration.photo')}</FormLabel>
            <FormControl>
              <FileUploader
                value={field.value}
                onValueChange={field.onChange}
                maxFileCount={1}
                maxSize={4 * 1024 * 1024}
                // progresses={progresses}
                // pass the onUpload function here for direct upload
                // onUpload={uploadFiles}
                // disabled={isUploading}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="type"
        render={({ field }) => (
          <FormItem className="space-y-3">
            <FormLabel tooltip={t('knowledgeConfiguration.permissionsTip')}>
              {t('knowledgeConfiguration.permissions')}
            </FormLabel>
            <FormControl>
              <RadioGroup
                onValueChange={field.onChange}
                defaultValue={field.value}
                className="flex flex-col space-y-1"
              >
                <FormItem className="flex items-center space-x-3 space-y-0">
                  <FormControl>
                    <RadioGroupItem value="me" />
                  </FormControl>
                  <FormLabel className="font-normal">
                    {t('knowledgeConfiguration.me')}
                  </FormLabel>
                </FormItem>
                <FormItem className="flex items-center space-x-3 space-y-0">
                  <FormControl>
                    <RadioGroupItem value="team" />
                  </FormControl>
                  <FormLabel className="font-normal">
                    {t('knowledgeConfiguration.team')}
                  </FormLabel>
                </FormItem>
              </RadioGroup>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </FormContainer>
  );
}
