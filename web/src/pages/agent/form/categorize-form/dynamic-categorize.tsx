import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { BlurTextarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { PlusOutlined } from '@ant-design/icons';
import { useUpdateNodeInternals } from '@xyflow/react';
import humanId from 'human-id';
import trim from 'lodash/trim';
import { ChevronsUpDown, X } from 'lucide-react';
import {
  ChangeEventHandler,
  FocusEventHandler,
  memo,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { UseFormReturn, useFieldArray, useFormContext } from 'react-hook-form';
import { v4 as uuid } from 'uuid';
import { z } from 'zod';
import useGraphStore from '../../store';
import DynamicExample from './dynamic-example';
import { useCreateCategorizeFormSchema } from './use-form-schema';

interface IProps {
  nodeId?: string;
}

interface INameInputProps {
  value?: string;
  onChange?: (value: string) => void;
  otherNames?: string[];
  validate(error?: string): void;
}

const getOtherFieldValues = (
  form: UseFormReturn,
  formListName: string = 'items',
  index: number,
  latestField: string,
) =>
  (form.getValues(formListName) ?? [])
    .map((x: any) => x[latestField])
    .filter(
      (x: string) =>
        x !== form.getValues(`${formListName}.${index}.${latestField}`),
    );

const InnerNameInput = ({
  value,
  onChange,
  otherNames,
  validate,
}: INameInputProps) => {
  const [name, setName] = useState<string | undefined>();
  const { t } = useTranslate('flow');

  const handleNameChange: ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      const val = e.target.value;
      setName(val);
      const trimmedVal = trim(val);
      // trigger validation
      if (otherNames?.some((x) => x === trimmedVal)) {
        validate(t('nameRepeatedMsg'));
      } else if (trimmedVal === '') {
        validate(t('nameRequiredMsg'));
      } else {
        validate('');
      }
    },
    [otherNames, validate, t],
  );

  const handleNameBlur: FocusEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      const val = e.target.value;
      if (otherNames?.every((x) => x !== val) && trim(val) !== '') {
        onChange?.(val);
      }
    },
    [onChange, otherNames],
  );

  useEffect(() => {
    setName(value);
  }, [value]);

  return (
    <Input
      value={name}
      onChange={handleNameChange}
      onBlur={handleNameBlur}
    ></Input>
  );
};

const NameInput = memo(InnerNameInput);

const InnerFormSet = ({ index }: IProps & { index: number }) => {
  const form = useFormContext();
  const { t } = useTranslate('flow');

  const buildFieldName = useCallback(
    (name: string) => {
      return `items.${index}.${name}`;
    },
    [index],
  );

  return (
    <section className="space-y-4">
      <FormField
        control={form.control}
        name={buildFieldName('name')}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('categoryName')}</FormLabel>
            <FormControl>
              <NameInput
                {...field}
                otherNames={getOtherFieldValues(form, 'items', index, 'name')}
                validate={(error?: string) => {
                  const fieldName = buildFieldName('name');
                  if (error) {
                    form.setError(fieldName, { message: error });
                  } else {
                    form.clearErrors(fieldName);
                  }
                }}
              ></NameInput>
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={buildFieldName('description')}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('description')}</FormLabel>
            <FormControl>
              <BlurTextarea {...field} rows={3} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {/* Create a hidden field to make Form instance record this */}
      <FormField
        control={form.control}
        name={'uuid'}
        render={() => <div></div>}
      />
      <DynamicExample name={buildFieldName('examples')}></DynamicExample>
    </section>
  );
};

const FormSet = memo(InnerFormSet);

const DynamicCategorize = ({ nodeId }: IProps) => {
  const updateNodeInternals = useUpdateNodeInternals();
  const FormSchema = useCreateCategorizeFormSchema();

  const deleteCategorizeCaseEdges = useGraphStore(
    (state) => state.deleteEdgesBySourceAndSourceHandle,
  );
  const form = useFormContext<z.infer<typeof FormSchema>>();
  const { t } = useTranslate('flow');
  const { fields, remove, append } = useFieldArray({
    name: 'items',
    control: form.control,
  });

  const handleAdd = useCallback(() => {
    append({
      name: humanId(),
      description: '',
      uuid: uuid(),
      examples: [{ value: '' }],
    });
    if (nodeId) updateNodeInternals(nodeId);
  }, [append, nodeId, updateNodeInternals]);

  const handleRemove = useCallback(
    (index: number) => () => {
      remove(index);
      if (nodeId) {
        const uuid = fields[index].uuid;
        deleteCategorizeCaseEdges(nodeId, uuid);
      }
    },
    [deleteCategorizeCaseEdges, fields, nodeId, remove],
  );

  return (
    <div className="flex flex-col gap-4 ">
      {fields.map((field, index) => (
        <Collapsible key={field.id} defaultOpen>
          <div className="flex items-center justify-between space-x-4">
            <h4 className="font-bold">
              {form.getValues(`items.${index}.name`)}
            </h4>
            <CollapsibleTrigger asChild>
              <div className="flex gap-4">
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-9 p-0"
                  onClick={handleRemove(index)}
                >
                  <X className="h-4 w-4" />
                </Button>
                <Button variant="ghost" size="sm" className="w-9 p-0">
                  <ChevronsUpDown className="h-4 w-4" />
                  <span className="sr-only">Toggle</span>
                </Button>
              </div>
            </CollapsibleTrigger>
          </div>
          <CollapsibleContent>
            <FormSet nodeId={nodeId} index={index}></FormSet>
          </CollapsibleContent>
        </Collapsible>
      ))}

      <Button type={'button'} onClick={handleAdd}>
        <PlusOutlined />
        {t('addCategory')}
      </Button>
    </div>
  );
};

export default memo(DynamicCategorize);
