import { Collapse } from '@/components/collapse';
import { LayoutRecognizeFormField } from '@/components/layout-recognize-form-field';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Switch } from '@/components/ui/switch';
import { FormTooltip } from '@/components/ui/tooltip';
import { zodResolver } from '@hookform/resolvers/zod';
import { Plus } from 'lucide-react';
import { memo, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BeginQueryType } from '../../constant';
import { BeginQuery, INextOperatorForm } from '../../interface';
import { ParameterDialog } from '../begin-form/parameter-dialog';
import { QueryTable } from '../begin-form/query-table';
import { useEditQueryRecord } from '../begin-form/use-edit-query';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

function UserFillUpForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const values = useValues(node);

  const FormSchema = z.object({
    enable_tips: z.boolean().optional(),
    tips: z.string().trim().optional(),
    layout_recognize: z.string().optional(),
    inputs: z
      .array(
        z.object({
          key: z.string(),
          type: z.string(),
          value: z.string(),
          optional: z.boolean(),
          name: z.string(),
          options: z.array(z.union([z.number(), z.string(), z.boolean()])),
        }),
      )
      .optional(),
  });

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  const inputs: BeginQuery[] = useWatch({
    control: form.control,
    name: 'inputs',
  });

  const hasFileInput = useMemo(
    () => inputs?.some((x) => x.type === BeginQueryType.File),
    [inputs],
  );

  const outputList = inputs?.map((item) => ({
    title: item.name,
    type: item.type,
  }));

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

  return (
    <section className="px-5 space-y-5">
      <Form {...form}>
        <FormField
          control={form.control}
          name={'enable_tips'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.openingSwitchTip')}>
                {t('flow.guidingQuestion')}
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

        <FormField
          control={form.control}
          name={'tips'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('chat.setAnOpenerTip')}>
                {t('flow.msg')}
              </FormLabel>
              <FormControl>
                <PromptEditor value={field.value} onChange={field.onChange} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        {/* Create a hidden field to make Form instance record this */}
        <FormField
          control={form.control}
          name={'inputs'}
          render={() => <div></div>}
        />
        <Collapse
          title={
            <div>
              {t('flow.input')}
              <FormTooltip tooltip={t('flow.beginInputTip')}></FormTooltip>
            </div>
          }
          rightContent={
            <Button
              variant={'ghost'}
              onClick={(e) => {
                e.preventDefault();
                showModal();
              }}
            >
              <Plus />
            </Button>
          }
        >
          <QueryTable
            data={inputs}
            showModal={showModal}
            deleteRecord={handleDeleteRecord}
          ></QueryTable>
        </Collapse>

        {visible && (
          <ParameterDialog
            hideModal={hideModal}
            initialValue={currentRecord}
            otherThanCurrentQuery={otherThanCurrentQuery}
            submit={ok}
          ></ParameterDialog>
        )}
        {hasFileInput && (
          <LayoutRecognizeFormField
            name="layout_recognize"
            horizontal={false}
            showMineruOptions={false}
            showPaddleocrOptions={false}
          ></LayoutRecognizeFormField>
        )}
      </Form>
      <Output list={outputList}></Output>
    </section>
  );
}

export default memo(UserFillUpForm);
