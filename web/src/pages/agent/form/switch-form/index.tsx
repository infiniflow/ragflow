import { FormContainer } from '@/components/form-container';
import { IconFont } from '@/components/icon-font';
import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { ISwitchForm } from '@/interfaces/database/flow';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { useMemo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  Operator,
  SwitchLogicOperatorOptions,
  SwitchOperatorOptions,
} from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';
import {
  useBuildComponentIdAndBeginOptions,
  useBuildVariableOptions,
} from '../../hooks/use-get-begin-query';
import { IOperatorForm } from '../../interface';
import { useValues } from './use-values';

const ConditionKey = 'conditions';
const ItemKey = 'items';

type ConditionCardsProps = {
  name: string;
} & IOperatorForm;

function ConditionCards({ name: parentName, node }: ConditionCardsProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  const componentIdOptions = useBuildComponentIdAndBeginOptions(
    node?.id,
    node?.parentId,
  );

  const switchOperatorOptions = useMemo(() => {
    return SwitchOperatorOptions.map((x) => ({
      value: x.value,
      icon: (
        <IconFont
          name={x.icon}
          className={cn('size-4', { 'rotate-180': x.value === '>' })}
        ></IconFont>
      ),
      label: t(`flow.switchOperatorOptions.${x.label}`),
    }));
  }, [t]);

  const name = `${parentName}.${ItemKey}`;

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  return (
    <section className="flex-1 space-y-2.5">
      {fields.map((field, index) => {
        return (
          <div key={field.id} className="flex">
            <Card className="bg-transparent border-input-border border flex-1">
              <section className="p-2 bg-background-card flex justify-between items-center">
                <FormField
                  control={form.control}
                  name={`${name}.${index}.cpn_id`}
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <RAGFlowSelect
                          {...field}
                          options={componentIdOptions}
                          placeholder={t('common.pleaseSelect')}
                          triggerClassName="w-30 text-background-checked bg-transparent border-none"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className="flex items-center">
                  <Separator orientation="vertical" className="h-2.5" />
                  <FormField
                    control={form.control}
                    name={`${name}.${index}.operator`}
                    render={({ field }) => (
                      <FormItem>
                        <FormControl>
                          <RAGFlowSelect
                            {...field}
                            options={switchOperatorOptions}
                            onlyShowSelectedIcon
                            triggerClassName="w-30 bg-transparent border-none"
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </section>
              <CardContent className="p-4 ">
                <FormField
                  control={form.control}
                  name={`${name}.${index}.value`}
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <Textarea {...field} className="bg-transparent" />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </CardContent>
            </Card>
            <Button variant={'ghost'} onClick={() => remove(index)}>
              <X />
            </Button>
          </div>
        );
      })}
      <div className="pr-9">
        <BlockButton
          className="mt-6"
          onClick={() => append({ operator: switchOperatorOptions[0].value })}
        >
          add
        </BlockButton>
      </div>
    </section>
  );
}

const SwitchForm = ({ node }: IOperatorForm) => {
  const { t } = useTranslation();
  const values = useValues();

  const FormSchema = z.object({
    conditions: z.array(
      z
        .object({
          logical_operator: z.string(),
          items: z
            .array(
              z.object({
                cpn_id: z.string(),
                operator: z.string(),
                value: z.string().optional(),
              }),
            )
            .optional(),
          to: z.array(z.string()).optional(),
        })
        .optional(),
    ),
  });

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const { fields, remove, append } = useFieldArray({
    name: ConditionKey,
    control: form.control,
  });

  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Switch,
    node?.id,
  );

  const getSelectedConditionTos = () => {
    const conditions: ISwitchForm['conditions'] = form?.getValues('conditions');

    return conditions?.filter((x) => !!x).map((x) => x?.to) ?? [];
  };

  const switchLogicOperatorOptions = useMemo(() => {
    return SwitchLogicOperatorOptions.map((x) => ({
      value: x,
      label: t(`flow.switchLogicOperatorOptions.${x}`),
    }));
  }, [t]);

  const componentIdOptions = useBuildVariableOptions(node?.id, node?.parentId);

  return (
    <Form {...form}>
      <form
        className="space-y-6 p-5 "
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        {fields.map((field, index) => {
          return (
            <FormContainer key={field.id} className="">
              <div>IF</div>
              <section className="flex items-center gap-2 !mt-2">
                <FormField
                  control={form.control}
                  name={`${ConditionKey}.${index}.logical_operator`}
                  render={({ field }) => (
                    <FormItem>
                      <FormControl>
                        <RAGFlowSelect
                          {...field}
                          options={switchLogicOperatorOptions}
                          triggerClassName="w-18"
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <ConditionCards
                  name={`${ConditionKey}.${index}`}
                ></ConditionCards>
              </section>
            </FormContainer>
          );
        })}
        <BlockButton
          onClick={() =>
            append({ logical_operator: SwitchLogicOperatorOptions[0] })
          }
        >
          add
        </BlockButton>
      </form>
    </Form>
  );
};

export default SwitchForm;
