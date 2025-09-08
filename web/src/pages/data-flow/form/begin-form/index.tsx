import { Collapse } from '@/components/collapse';
import { Button } from '@/components/ui/button';
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
import { FormTooltip } from '@/components/ui/tooltip';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { Plus } from 'lucide-react';
import { memo, useEffect, useRef } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { AgentDialogueMode } from '../../constant';
import { INextOperatorForm } from '../../interface';
import { ParameterDialog } from './parameter-dialog';
import { QueryTable } from './query-table';
import { useEditQueryRecord } from './use-edit-query';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const ModeOptions = [
  { value: AgentDialogueMode.Conversational, label: t('flow.conversational') },
  { value: AgentDialogueMode.Task, label: t('flow.task') },
];

function BeginForm({ node }: INextOperatorForm) {
  const { t } = useTranslation();

  const values = useValues(node);

  const FormSchema = z.object({
    enablePrologue: z.boolean().optional(),
    prologue: z.string().trim().optional(),
    mode: z.string(),
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

  const inputs = useWatch({ control: form.control, name: 'inputs' });
  const mode = useWatch({ control: form.control, name: 'mode' });

  const enablePrologue = useWatch({
    control: form.control,
    name: 'enablePrologue',
  });

  const previousModeRef = useRef(mode);

  useEffect(() => {
    if (
      previousModeRef.current === AgentDialogueMode.Task &&
      mode === AgentDialogueMode.Conversational
    ) {
      form.setValue('enablePrologue', true);
    }
    previousModeRef.current = mode;
  }, [mode, form]);

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
          name={'mode'}
          render={({ field }) => (
            <FormItem>
              <FormLabel tooltip={t('flow.modeTip')}>
                {t('flow.mode')}
              </FormLabel>
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
        {mode === AgentDialogueMode.Conversational && (
          <FormField
            control={form.control}
            name={'enablePrologue'}
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('flow.openingSwitchTip')}>
                  {t('flow.openingSwitch')}
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
        )}
        {mode === AgentDialogueMode.Conversational && enablePrologue && (
          <FormField
            control={form.control}
            name={'prologue'}
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('chat.setAnOpenerTip')}>
                  {t('flow.openingCopy')}
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
      </Form>
    </section>
  );
}

export default memo(BeginForm);
