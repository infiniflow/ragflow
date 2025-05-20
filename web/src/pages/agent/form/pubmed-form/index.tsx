import { TopNFormField } from '@/components/top-n-item';
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
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

const PubMedForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslate('flow');

  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <TopNFormField></TopNFormField>
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('emailTip')}>{t('email')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
};

export default PubMedForm;
