import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  QWeatherLangOptions,
  QWeatherTimePeriodOptions,
  QWeatherTypeOptions,
  QWeatherUserTypeOptions,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import { DynamicInputVariable } from '../components/next-dynamic-input-variable';

enum FormFieldName {
  Type = 'type',
  UserType = 'user_type',
}

const QWeatherForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslation();
  const typeValue = form.watch(FormFieldName.Type);

  const qWeatherLangOptions = useMemo(() => {
    return QWeatherLangOptions.map((x) => ({
      value: x,
      label: t(`flow.qWeatherLangOptions.${x}`),
    }));
  }, [t]);

  const qWeatherTypeOptions = useMemo(() => {
    return QWeatherTypeOptions.map((x) => ({
      value: x,
      label: t(`flow.qWeatherTypeOptions.${x}`),
    }));
  }, [t]);

  const qWeatherUserTypeOptions = useMemo(() => {
    return QWeatherUserTypeOptions.map((x) => ({
      value: x,
      label: t(`flow.qWeatherUserTypeOptions.${x}`),
    }));
  }, [t]);

  const getQWeatherTimePeriodOptions = useCallback(() => {
    let options = QWeatherTimePeriodOptions;
    const userType = form.getValues(FormFieldName.UserType);
    if (userType === 'free') {
      options = options.slice(0, 3);
    }
    return options.map((x) => ({
      value: x,
      label: t(`flow.qWeatherTimePeriodOptions.${x}`),
    }));
  }, [form, t]);

  return (
    <Form {...form}>
      <form
        className="space-y-6"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <DynamicInputVariable node={node}></DynamicInputVariable>
        <FormField
          control={form.control}
          name="web_apikey"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.webApiKey')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="lang"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.lang')}</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={qWeatherLangOptions}
                ></RAGFlowSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name={FormFieldName.Type}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.type')}</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={qWeatherTypeOptions}
                ></RAGFlowSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name={FormFieldName.UserType}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.userType')}</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  {...field}
                  options={qWeatherUserTypeOptions}
                ></RAGFlowSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {typeValue === 'weather' && (
          <FormField
            control={form.control}
            name={'time_period'}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.timePeriod')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={getQWeatherTimePeriodOptions()}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
      </form>
    </Form>
  );
};

export default QWeatherForm;
