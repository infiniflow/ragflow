import { FormContainer } from '@/components/form-container';
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
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { X } from 'lucide-react';
import { memo, useCallback, useMemo } from 'react';
import { useFieldArray, useForm, useFormContext } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { SwitchLogicOperatorOptions } from '../../constant';
import { IOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { QueryVariable } from '../components/query-variable';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const ConditionKey = 'conditions';
const ItemKey = 'items';

type ConditionCardsProps = {
  name: string;
  removeParent(index: number): void;
  parentIndex: number;
  parentLength: number;
} & IOperatorForm;

function ConditionCards({
  name: parentName,
  parentIndex,
  removeParent,
  parentLength,
}: ConditionCardsProps) {
  const form = useFormContext();

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
    <section className="flex-1 space-y-2.5 min-w-0">
      {fields.map((field, index) => {
        return (
          <div key={field.id} className="flex">
            <Card
              className={cn(
                'relative bg-transparent border-input-border border flex-1 min-w-0',
                {
                  'before:w-10 before:absolute before:h-[1px] before:bg-input-border before:top-1/2 before:-left-10':
                    fields.length > 1 &&
                    (index === 0 || index === fields.length - 1),
                },
              )}
            >
              <section className="p-2 bg-bg-card flex justify-between items-center">
                <FormField
                  control={form.control}
                  name={`${name}.${index}.cpn_id`}
                  render={({ field }) => (
                    <FormItem className="flex-1 min-w-0">
                      <FormControl>
                        <QueryVariable
                          pureQuery
                          {...field}
                          hideLabel
                        ></QueryVariable>
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
          {t('common.add')}
        </BlockButton>
      </div>
    </section>
  );
}

function SwitchForm({ node }: IOperatorForm) {
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

  const switchLogicOperatorOptions = useMemo(() => {
    return SwitchLogicOperatorOptions.map((x) => ({
      value: x,
      label: t(`flow.switchLogicOperatorOptions.${x}`),
    }));
  }, [t]);

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        {fields.map((field, index) => {
          const name = `${ConditionKey}.${index}`;
          const conditions: Array<any> = form.getValues(`${name}.${ItemKey}`);
          const conditionLength = conditions.length;
          return (
            <FormContainer key={field.id} className="">
              <div className="flex justify-between items-center">
                <section>
                  <span>{index === 0 ? 'IF' : 'ELSEIF'}</span>
                  <div className="text-text-secondary">Case {index + 1}</div>
                </section>
                {index !== 0 && (
                  <Button
                    variant={'secondary'}
                    className="-translate-y-1"
                    onClick={() => remove(index)}
                  >
                    {t('common.remove')} <X />
                  </Button>
                )}
              </div>
              <section className="flex gap-2 !mt-2 relative">
                {conditionLength > 1 && (
                  <section className="flex flex-col w-[72px]">
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
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <div className="relative  w-1 flex-1 before:absolute before:w-[1px]  before:bg-input-border before:top-0 before:bottom-36 before:left-10"></div>
                  </section>
                )}
                <ConditionCards
                  name={name}
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
              to: [],
            })
          }
        >
          {t('common.add')}
        </BlockButton>
      </FormWrapper>
    </Form>
  );
}

export default memo(SwitchForm);
