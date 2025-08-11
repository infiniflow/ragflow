import { z } from 'zod';

import {
  FileUpload,
  FileUploadDropzone,
  FileUploadItem,
  FileUploadItemDelete,
  FileUploadItemMetadata,
  FileUploadItemPreview,
  FileUploadList,
  FileUploadTrigger,
} from '@/components/file-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Form, FormControl, FormItem, FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { transformBase64ToFile } from '@/utils/file-util';
import { zodResolver } from '@hookform/resolvers/zod';
import { CloudUpload, X } from 'lucide-react';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';

const formSchema = z.object({
  title: z.string().min(1, {}),
  avatar: z.array(z.custom<File>()).optional().nullable(),
  description: z.string().optional().nullable(),
  permission: z.string(),
});

export type SettingFormSchemaType = z.infer<typeof formSchema>;

export const AgentSettingId = 'agentSettingId';

type SettingFormProps = {
  submit: (values: SettingFormSchemaType) => void;
};

export function SettingForm({ submit }: SettingFormProps) {
  const { t } = useTranslate('flow.settings');
  const { data } = useFetchAgent();

  const form = useForm<SettingFormSchemaType>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      title: '',
      permission: 'me',
    },
  });

  useEffect(() => {
    form.reset({
      title: data?.title,
      description: data?.description,
      avatar: data.avatar ? [transformBase64ToFile(data.avatar)] : [],
      permission: data?.permission,
    });
  }, [data, form]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(submit)}
        className="space-y-8"
        id={AgentSettingId}
      >
        <RAGFlowFormItem name="title" label={t('title')}>
          <Input />
        </RAGFlowFormItem>
        <RAGFlowFormItem name="avatar" label={t('photo')}>
          {(field) => (
            <FileUpload
              value={field.value}
              onValueChange={field.onChange}
              accept="image/*"
              maxFiles={1}
              onFileReject={(_, message) => {
                form.setError('avatar', {
                  message,
                });
              }}
              multiple
            >
              <FileUploadDropzone className="flex-row flex-wrap border-dotted text-center">
                <CloudUpload className="size-4" />
                Drag and drop or
                <FileUploadTrigger asChild>
                  <Button variant="link" size="sm" className="p-0">
                    choose files
                  </Button>
                </FileUploadTrigger>
                to upload
              </FileUploadDropzone>
              <FileUploadList>
                {field.value?.map((file: File, index: number) => (
                  <FileUploadItem key={index} value={file}>
                    <FileUploadItemPreview />
                    <FileUploadItemMetadata />
                    <FileUploadItemDelete asChild>
                      <Button variant="ghost" size="icon" className="size-7">
                        <X />
                        <span className="sr-only">Delete</span>
                      </Button>
                    </FileUploadItemDelete>
                  </FileUploadItem>
                ))}
              </FileUploadList>
            </FileUpload>
          )}
        </RAGFlowFormItem>
        <RAGFlowFormItem name="description" label={t('description')}>
          <Textarea rows={4} />
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="permission"
          label={t('permissions')}
          tooltip={t('permissionsTip')}
        >
          {(field) => (
            <RadioGroup
              onValueChange={field.onChange}
              value={field.value}
              className="flex"
            >
              <FormItem className="flex items-center gap-3">
                <FormControl>
                  <RadioGroupItem value="me" id="me" />
                </FormControl>
                <FormLabel
                  className="font-normal !m-0 cursor-pointer"
                  htmlFor="me"
                >
                  {t('me')}
                </FormLabel>
              </FormItem>

              <FormItem className="flex items-center gap-3">
                <FormControl>
                  <RadioGroupItem value="team" id="team" />
                </FormControl>
                <FormLabel
                  className="font-normal !m-0 cursor-pointer"
                  htmlFor="team"
                >
                  {t('team')}
                </FormLabel>
              </FormItem>
            </RadioGroup>
          )}
        </RAGFlowFormItem>
      </form>
    </Form>
  );
}
