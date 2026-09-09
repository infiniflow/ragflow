import { BlockButton, Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import { Textarea } from '@/components/ui/textarea';
import { SwitchLogicOperator } from '@/constants/agent';
import { useBuildSwitchOperatorOptions } from '@/hooks/logic-hooks/use-build-operator-options';
import { useBuildSwitchLogicOperatorOptions } from '@/hooks/logic-hooks/use-build-options';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { X } from 'lucide-react';
import { memo, useCallback, useMemo } from 'react';
import {
  useFieldArray,
  useForm,
  useFormContext,
  useWatch,
} from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { useFilterQueryVariableOptionsByTypes } from '../../hooks/use-get-begin-query';
import { IOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { GroupedSelectWithSecondaryMenu } from '../components/select-with-secondary-menu';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

/**
 * Split a stored cpn_id reference into the dropdown base (`cpn@root`) and the
 * dotted-path suffix (`field.subfield`). The canvas resolver in
 * `agent/canvas.py` (`Graph.get_variable_value`) consumes the combined string;
 * the suffix lets the Switch condition target nested fields of an object
 * output instead of the whole output. See issue #14235.
 */
function splitBaseAndPath(value: string): { base: string; suffix: string } {
  if (!value) return { base: '', suffix: '' };
  const atIdx = value.indexOf('@');
  if (atIdx < 0) {
    // Globals / env vars / sys vars have no '@' — they're leaf values and
    // cannot be drilled into via Switch, so the whole string is the base.
    return { base: value, suffix: '' };
  }
  const rest = value.slice(atIdx + 1);
  const dotIdx = rest.indexOf('.');
  if (dotIdx < 0) return { base: value, suffix: '' };
  return {
    base: value.slice(0, atIdx + 1 + dotIdx),
    suffix: rest.slice(dotIdx + 1),
  };
}

/** Combine a dropdown base and a user-typed dotted suffix into a single ref. */
function joinBaseAndPath(base: string, suffix: string): string {
  const trimmed = suffix.trim().replace(/^\.+/, '');
  if (!base) return trimmed;
  return trimmed ? `${base}.${trimmed}` : base;
}

type VariableWithPathProps = {
  name: string;
};

/**
 * Switch-condition variable picker: a dropdown for the upstream variable plus
 * an optional `.field.subfield` text input. Both controls write to a single
 * `cpn_id` form field, joined as `cpn@root.field.subfield`.
 */
function VariableWithPath({ name }: VariableWithPathProps) {
  const { t: translate } = useTranslation();
  const form = useFormContext();
  const fullValue: string =
    useWatch({ control: form.control, name }) ?? '';

  const { base, suffix } = useMemo(
    () => splitBaseAndPath(fullValue),
    [fullValue],
  );

  const options = useFilterQueryVariableOptionsByTypes({ types: [] });

  const writeCombined = useCallback(
    (nextBase: string, nextSuffix: string) => {
      // Globals/env/sys vars (no '@') are leaf values — drilling a dotted
      // path into them would produce an invalid ref the canvas resolver
      // can't look up. Drop any stale suffix when the base isn't drillable.
      const safeSuffix = nextBase.includes('@') ? nextSuffix : '';
      form.setValue(name, joinBaseAndPath(nextBase, safeSuffix), {
        shouldDirty: true,
        shouldValidate: true,
      });
    },
    [form, name],
  );

  const isDrillable = base.includes('@');

  return (
    <div className="flex-1 min-w-0 flex items-center gap-2">
      <div className="flex-[3] min-w-0">
        <GroupedSelectWithSecondaryMenu
          options={options}
          value={base}
          onChange={(val) => writeCombined(val, suffix)}
        />
      </div>
      <Input
        className="flex-[2] min-w-0 bg-transparent"
        placeholder={translate('flow.fieldPathPlaceholder', {
          defaultValue: '.field.subfield (optional)',
        })}
        value={suffix}
        disabled={!isDrillable}
        onChange={(e) => writeCombined(base, e.target.value)}
      />
    </div>
  );
}

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
              <section className="p-2 bg-bg-card flex justify-between items-center gap-2">
                <FormField
                  control={form.control}
                  name={`${name}.${index}.cpn_id`}
                  render={() => (
                    <FormItem className="flex-1 min-w-0">
                      <FormControl>
                        <VariableWithPath
                          name={`${name}.${index}.cpn_id`}
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

  const switchLogicOperatorOptions = useBuildSwitchLogicOperatorOptions();

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        {fields.map((field, index) => {
          const name = `${ConditionKey}.${index}`;
          const conditions: Array<any> = form.getValues(`${name}.${ItemKey}`);
          const conditionLength = conditions.length;
          return (
            <section key={field.id} className="space-y-5">
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
              <Separator />
            </section>
          );
        })}
        <BlockButton
          onClick={() =>
            append({
              logical_operator: SwitchLogicOperator.And,
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
