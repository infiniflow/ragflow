import { BlockButton } from '@/components/ui/button';
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
import { Textarea } from '@/components/ui/textarea';
import { buildSelectOptions } from '@/utils/common-util';
import { useCallback } from 'react';
import { useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { AgentDialogueMode } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { ParameterDialog } from './parameter-dialog';
import QueryTable from './query-table';
import { useEditQueryRecord } from './use-edit-query';

const ModeOptions = buildSelectOptions([
  (AgentDialogueMode.Conversational, AgentDialogueMode.Task),
]);

const BeginForm = ({ form, node }: INextOperatorForm) => {
  const { t } = useTranslation();

  const query = useWatch({ control: form.control, name: 'query' });
  const enablePrologue = useWatch({
    control: form.control,
    name: 'enablePrologue',
  });

  const {
    ok,
    currentRecord,
    visible,
    hideModal,
    showModal,
    otherThanCurrentQuery,
    handleDeleteRecord,
  } = useEditQueryRecord({
    form,
    node,
  });

  const handleParameterDialogSubmit = useCallback(
    (values: any) => {
      ok(values);
    },
    [ok],
  );

  return (
    <section className="px-5 space-y-5">
      <Form {...form}>
        <FormField
          control={form.control}
          name={'mode'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.setAnOpenerTip')}>Mode</FormLabel>
              <FormControl>
                <RAGFlowSelect
                  placeholder={t('common.pleaseSelect')}
                  options={ModeOptions}
                  {...field}
                ></RAGFlowSelect>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name={'enablePrologue'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.setAnOpenerTip')}>
                Welcome Message
              </FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {enablePrologue && (
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
        )}
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
        <BlockButton onClick={() => showModal()}>
          {t('flow.addItem')}
        </BlockButton>
        {visible && (
          <ParameterDialog
            visible={visible}
            hideModal={hideModal}
            initialValue={currentRecord}
            onOk={ok}
            otherThanCurrentQuery={otherThanCurrentQuery}
            submit={handleParameterDialogSubmit}
          ></ParameterDialog>
        )}
      </Form>
    </section>
  );
};

export default BeginForm;
