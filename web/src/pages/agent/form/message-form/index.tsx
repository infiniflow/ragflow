import { MemoriesFormField } from '@/components/memories-form-field';
import { BlockButton, Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { WebHookResponseStatusFormField } from '@/components/webhook-response-status';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { memo } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { ExportFileType } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { PromptEditor } from '../components/prompt-editor';
import { useShowWebhookResponseStatus } from './use-show-response-status';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

function MessageForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const values = useValues(node);

  const FormSchema = z.object({
    content: z
      .array(
        z.object({
          value: z.string(),
        }),
      )
      .optional(),
    output_format: z.string().optional(),
    auto_play: z.boolean().optional(),
    status: z.number().optional(),
    memory_ids: z.array(z.string()).optional(),
  });

  const form = useForm({
    defaultValues: {
      ...values,
      output_format: values.output_format,
      auto_play: values.auto_play,
    },
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const { fields, append, remove } = useFieldArray({
    name: 'content',
    control: form.control,
  });

  const { showWebhookResponseStatus, isWebhookMode } =
    useShowWebhookResponseStatus(form);

  return (
    <Form {...form}>
      <FormWrapper>
        {showWebhookResponseStatus && (
          <WebHookResponseStatusFormField name="status"></WebHookResponseStatusFormField>
        )}
        <FormItem>
          <FormLabel tooltip={t('flow.msgTip')}>{t('flow.msg')}</FormLabel>
          <div className="space-y-4">
            {fields.map((field, index) => (
              <div key={field.id} className="flex items-start gap-2">
                <FormField
                  control={form.control}
                  name={`content.${index}.value`}
                  render={({ field }) => (
                    <FormItem className="flex-1">
                      <FormControl>
                        <PromptEditor
                          {...field}
                          placeholder={t('flow.messagePlaceholder')}
                        ></PromptEditor>
                      </FormControl>
                    </FormItem>
                  )}
                />
                {fields.length > 1 && (
                  <Button
                    type="button"
                    variant={'ghost'}
                    onClick={() => remove(index)}
                  >
                    <X />
                  </Button>
                )}
              </div>
            ))}

            <BlockButton
              type="button"
              onClick={() => append({ value: '' })} // "" will cause the inability to add, refer to: https://github.com/orgs/react-hook-form/discussions/8485#discussioncomment-2961861
            >
              {t('flow.addMessage')}
            </BlockButton>
          </div>
          <FormMessage />
        </FormItem>
        {!isWebhookMode && (
          <>
            <FormItem>
              <FormLabel tooltip={t('flow.downloadFileTypeTip')}>
                {t('flow.downloadFileType')}
              </FormLabel>
              <FormField
                control={form.control}
                name={`output_format`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <RAGFlowSelect
                        options={Object.keys(ExportFileType).map(
                          (key: string) => {
                            return {
                              value:
                                ExportFileType[
                                  key as keyof typeof ExportFileType
                                ],
                              label: key,
                            };
                          },
                        )}
                        {...field}
                        onValueChange={field.onChange}
                        placeholder={t('common.selectPlaceholder')}
                        allowClear
                      ></RAGFlowSelect>
                    </FormControl>
                  </FormItem>
                )}
              />
            </FormItem>
            <FormItem>
              <FormLabel>{t('flow.autoPlay')}</FormLabel>
              <FormField
                control={form.control}
                name={`auto_play`}
                render={({ field }) => (
                  <FormItem className="flex-1">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </FormItem>
          </>
        )}
        <MemoriesFormField label={t('flow.saveToMemory')}></MemoriesFormField>
      </FormWrapper>
    </Form>
  );
}

export default memo(MessageForm);
