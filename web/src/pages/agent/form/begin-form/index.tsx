import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Textarea } from '@/components/ui/textarea';
import { useCallback } from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { BeginQuery, INextOperatorForm } from '../../interface';
import { useEditQueryRecord } from './hooks';
import { ParameterDialog } from './next-paramater-modal';
import QueryTable from './query-table';

const BeginForm = ({ form }: INextOperatorForm) => {
  const { t } = useTranslation();

  const query = useWatch({ control: form.control, name: 'query' });

  const {
    ok,
    currentRecord,
    visible,
    hideModal,
    showModal,
    otherThanCurrentQuery,
  } = useEditQueryRecord({
    form,
  });

  const handleDeleteRecord = useCallback(
    (idx: number) => {
      const query = form?.getValues('query') || [];
      const nextQuery = query.filter(
        (item: BeginQuery, index: number) => index !== idx,
      );
      // onValuesChange?.(
      //   { query: nextQuery },
      //   { query: nextQuery, prologue: form?.getFieldValue('prologue') },
      // );
    },
    [form],
  );

  return (
    <Form {...form}>
      <FormField
        control={form.control}
        name={'prologue'}
        render={({ field }) => (
          <FormItem>
            <FormLabel tooltip={t('chat.setAnOpenerTip')}>
              {t('chat.setAnOpener')}
            </FormLabel>
            <FormControl>
              <Textarea
                rows={5}
                {...field}
                placeholder={t('common.pleaseInput')}
              ></Textarea>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {/* Create a hidden field to make Form instance record this */}
      <FormField
        control={form.control}
        name={'query'}
        render={() => <div></div>}
      />

      <QueryTable
        data={query}
        showModal={showModal}
        deleteRecord={handleDeleteRecord}
      ></QueryTable>

      <Button onClick={() => showModal()}>{t('flow.addItem')}</Button>

      {visible && (
        <ParameterDialog
          visible={visible}
          hideModal={hideModal}
          initialValue={currentRecord}
          onOk={ok}
          otherThanCurrentQuery={otherThanCurrentQuery}
        ></ParameterDialog>
      )}
    </Form>
  );
};

export default BeginForm;
