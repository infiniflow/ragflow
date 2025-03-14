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
import { RAGFlowSelect } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { PlusOutlined } from '@ant-design/icons';
import { useUpdateNodeInternals } from '@xyflow/react';
import humanId from 'human-id';
import trim from 'lodash/trim';
import { ChevronsUpDown, X } from 'lucide-react';
import {
  ChangeEventHandler,
  FocusEventHandler,
  useCallback,
  useEffect,
  useState,
} from 'react';
import { UseFormReturn, useFieldArray, useFormContext } from 'react-hook-form';
import { Operator } from '../../constant';
import { useBuildFormSelectOptions } from '../../form-hooks';

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

const NameInput = ({
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

const FormSet = ({ nodeId, index }: IProps & { index: number }) => {
  const form = useFormContext();
  const { t } = useTranslate('flow');
  const buildCategorizeToOptions = useBuildFormSelectOptions(
    Operator.Categorize,
    nodeId,
  );

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
              <Textarea {...field} rows={3} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={buildFieldName('examples')}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('examples')}</FormLabel>
            <FormControl>
              <Textarea {...field} rows={3} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name={buildFieldName('to')}
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('nextStep')}</FormLabel>
            <FormControl>
              <RAGFlowSelect
                {...field}
                allowClear
                options={buildCategorizeToOptions(
                  getOtherFieldValues(form, 'items', index, 'to'),
                )}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="index"
        render={({ field }) => (
          <FormItem className="hidden">
            <FormLabel>{t('examples')}</FormLabel>
            <FormControl>
              <Input {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </section>
  );
};

const DynamicCategorize = ({ nodeId }: IProps) => {
  const updateNodeInternals = useUpdateNodeInternals();
  const form = useFormContext();
  const { t } = useTranslate('flow');
  const { fields, remove, append } = useFieldArray({
    name: 'items',
    control: form.control,
  });

  const handleAdd = () => {
    append({
      name: humanId(),
    });
    if (nodeId) updateNodeInternals(nodeId);
  };

  return (
    <div className="flex flex-col gap-4 ">
      {fields.map((field, index) => (
        <Collapsible key={field.id}>
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
                  onClick={() => remove(index)}
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

export default DynamicCategorize;
