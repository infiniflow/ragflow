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
import { ISwitchForm } from '@/interfaces/database/agent';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { X } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import {
  Operator,
  SwitchLogicOperatorOptions,
  SwitchOperatorOptions,
} from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';
import { useBuildComponentIdAndBeginOptions } from '../../hooks/use-get-begin-query';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { IOperatorForm } from '../../interface';
import { useValues } from './use-values';

const ConditionKey = 'conditions';
const ItemKey = 'items';

type ConditionCardsProps = {
  name: string;
  removeParent(index: number): void;
  parentIndex: number;
  parentLength: number;
} & IOperatorForm;

const OperatorIcon = function OperatorIcon({
  icon,
  value,
}: Omit<(typeof SwitchOperatorOptions)[0], 'label'>) {
  return (
    <IconFont
      name={icon}
      className={cn('size-4', {
        'rotate-180': value === '>',
      })}
    ></IconFont>
  );
};

function useBuildSwitchOperatorOptions() {
  const { t } = useTranslation();

  const switchOperatorOptions = useMemo(() => {
    return SwitchOperatorOptions.map((x) => ({
      value: x.value,
      icon: <OperatorIcon icon={x.icon} value={x.value}></OperatorIcon>,
      label: t(`flow.switchOperatorOptions.${x.label}`),
    }));
  }, [t]);

  return switchOperatorOptions;
}

function ConditionCards({
  name: parentName,
  node,
  parentIndex,
  removeParent,
  parentLength,
}: ConditionCardsProps) {
  const form = useFormContext();
  const { t } = useTranslation();

  const componentIdOptions = useBuildComponentIdAndBeginOptions(
    node?.id,
    node?.parentId,
  );

  const switchOperatorOptions = useBuildSwitchOperatorOptions();

  const name = `${parentName}.${ItemKey}`;

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  const handleRemove = useCallback(
    (index: number) => () => {
      remove(index);
      if (parentIndex !== 0 && index === 0 && parentLength === 1) {
        removeParent(parentIndex);
      }
    },
    [parentIndex, parentLength, remove, removeParent],
  );

  return (
    <section className="flex-1 space-y-2.5">
      {fields.map((field, index) => {
        return (
          <div key={field.id} className="flex">
            <Card
              className={cn(
                'relative bg-transparent border-input-border border flex-1 ',
                {
                  'before:w-10 before:absolute before:h-[1px] before:bg-input-border before:top-1/2 before:-left-10':
                    index === 0 || index === fields.length - 1,
                },
              )}
            >
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
            <Button variant={'ghost'} onClick={handleRemove(index)}>
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
          Add
        </BlockButton>
      </div>
    </section>
  );
}

const SwitchForm = ({ node }: IOperatorForm) => {
  const { t } = useTranslation();
  const values = useValues(node);
  const switchOperatorOptions = useBuildSwitchOperatorOptions();

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

  useWatchFormChange(node?.id, form);

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
              <div>{index === 0 ? 'IF' : 'ELSEIF'}</div>
              <section className="flex  gap-2 !mt-2 relative">
                <section className="flex flex-col">
                  <div className="relative  w-1 flex-1 before:absolute before:w-[1px]  before:bg-input-border before:top-20 before:bottom-0 before:left-10"></div>
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
                  <div className="relative  w-1 flex-1 before:absolute before:w-[1px]  before:bg-input-border before:top-0 before:bottom-36 before:left-10"></div>
                </section>
                <ConditionCards
                  name={`${ConditionKey}.${index}`}
                  removeParent={remove}
                  parentIndex={index}
                  parentLength={fields.length}
                ></ConditionCards>
              </section>
            </FormContainer>
          );
        })}
        <BlockButton
          onClick={() =>
            append({
              logical_operator: SwitchLogicOperatorOptions[0],
              [ItemKey]: [
                {
                  operator: switchOperatorOptions[0].value,
                },
              ],
            })
          }
        >
          Add
        </BlockButton>
      </form>
    </Form>
  );
};

export default SwitchForm;
