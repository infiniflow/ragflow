import { z } from 'zod';

import { AvatarUpload } from '@/components/avatar-upload';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Form, FormControl, FormItem, FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { zodResolver } from '@hookform/resolvers/zod';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';

const formSchema = z.object({
  title: z.string().min(1, {}),
  avatar: z.string().optional().nullable(),
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
      avatar: data.avatar,
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
          <AvatarUpload></AvatarUpload>
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
