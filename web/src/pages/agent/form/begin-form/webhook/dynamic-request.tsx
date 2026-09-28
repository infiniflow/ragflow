import { KeyInput } from '@/components/key-input';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { buildOptions } from '@/utils/form';
import { loader } from '@monaco-editor/react';
import { X } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { TypesWithArray, WebhookRequestParameters } from '../../../constant';
import { DynamicFormHeader } from '../../components/dynamic-fom-header';

loader.config({ paths: { vs: '/vs' } });

type SelectKeysProps = {
  name: string;
  label: ReactNode;
  tooltip?: string;
  keyField?: string;
  operatorField?: string;
  requiredField?: string;
  nodeId?: string;
  isObject?: boolean;
  operatorList: WebhookRequestParameters[];
};

export function DynamicRequest({
  name,
  label,
  tooltip,
  keyField = 'key',
  operatorField = 'type',
  requiredField = 'required',
  operatorList,
}: SelectKeysProps) {
  const form = useFormContext();

  const { fields, remove, append } = useFieldArray({
    name: name,
    control: form.control,
  });

  // append is not a change event, so trigger manually to surface
  // the required/duplicate errors of the freshly added empty row.
  const handleAdd = useCallback(() => {
    append({
      [keyField]: '',
      [operatorField]: TypesWithArray.String,
      [requiredField]: false,
    });
    form.trigger(name);
  }, [append, form, keyField, name, operatorField, requiredField]);

  return (
    <section className="space-y-2">
      <DynamicFormHeader
        label={label}
        tooltip={tooltip}
        onClick={handleAdd}
      ></DynamicFormHeader>
      <div className="space-y-5">
        {fields.map((field, index) => {
          const keyFieldAlias = `${name}.${index}.${keyField}`;
          const operatorFieldAlias = `${name}.${index}.${operatorField}`;
          const requiredFieldAlias = `${name}.${index}.${requiredField}`;

          return (
            <section key={field.id} className="flex gap-2">
              <div className="flex-1 space-y-3 min-w-0">
                <div className="flex items-start gap-2">
                  <RAGFlowFormItem name={keyFieldAlias} className="flex-1 ">
                    <KeyInput></KeyInput>
                  </RAGFlowFormItem>
                  <div className="flex h-8 items-center">
                    <Separator className="w-2" />
                  </div>
                  <RAGFlowFormItem name={operatorFieldAlias} className="flex-1">
                    {(field) => (
                      <SelectWithSearch
                        value={field.value}
                        onChange={(val) => {
                          field.onChange(val);
                        }}
                        options={buildOptions(operatorList)}
                      ></SelectWithSearch>
                    )}
                  </RAGFlowFormItem>
                  <div className="flex h-8 items-center">
                    <Separator className="w-2" />
                  </div>
                  <RAGFlowFormItem name={requiredFieldAlias}>
                    {(field) => (
                      <div className="flex h-8 items-center">
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        ></Switch>
                      </div>
                    )}
                  </RAGFlowFormItem>
                </div>
              </div>

              <Button variant={'ghost'} onClick={() => remove(index)}>
                <X />
              </Button>
            </section>
          );
        })}
      </div>
    </section>
  );
}
