import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  CrawlerExtractTypeFormField,
  CrawlerFormSchema,
  CrawlerProxyFormField,
} from '../../crawler-form';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

export const FormSchema = z.object({
  ...CrawlerFormSchema,
});

const CrawlerForm = () => {
  const defaultValues = useValues();

  const form = useForm({
    defaultValues: defaultValues,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-4"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <CrawlerProxyFormField></CrawlerProxyFormField>
        <CrawlerExtractTypeFormField></CrawlerExtractTypeFormField>
      </form>
    </Form>
  );
};

export default CrawlerForm;
