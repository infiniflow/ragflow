import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { ReactNode } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialEmailValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';

interface InputFormFieldProps {
  name: string;
  label: ReactNode;
  type?: string;
}

function InputFormField({ name, label, type }: InputFormFieldProps) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input {...field} type={type}></Input>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}

function PromptFormField({ name, label }: InputFormFieldProps) {
  const form = useFormContext();

  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <PromptEditor
              {...field}
              showToolbar={false}
              multiLine={false}
            ></PromptEditor>
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
export function EmailFormWidgets() {
  const { t } = useTranslate('flow');

  return (
    <>
      <InputFormField
        name="smtp_server"
        label={t('smtpServer')}
      ></InputFormField>
      <InputFormField
        name="smtp_port"
        label={t('smtpPort')}
        type="number"
      ></InputFormField>
      <InputFormField name="email" label={t('senderEmail')}></InputFormField>
      <InputFormField
        name="password"
        label={t('authCode')}
        type="password"
      ></InputFormField>
      <InputFormField
        name="sender_name"
        label={t('senderName')}
      ></InputFormField>
    </>
  );
}

export const EmailFormPartialSchema = {
  smtp_server: z.string(),
  smtp_port: z.number(),
  email: z.string(),
  password: z.string(),
  sender_name: z.string(),
};

const FormSchema = z.object({
  to_email: z.string(),
  cc_email: z.string(),
  content: z.string(),
  subject: z.string(),
  ...EmailFormPartialSchema,
});

const outputList = buildOutputList(initialEmailValues.outputs);

const EmailForm = ({ node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');
  const defaultValues = useFormValues(initialEmailValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <PromptFormField
            name="to_email"
            label={t('toEmail')}
          ></PromptFormField>
          <PromptFormField
            name="cc_email"
            label={t('ccEmail')}
          ></PromptFormField>
          <PromptFormField
            name="content"
            label={t('content')}
          ></PromptFormField>
          <PromptFormField
            name="subject"
            label={t('subject')}
          ></PromptFormField>
          <EmailFormWidgets></EmailFormWidgets>
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default EmailForm;
