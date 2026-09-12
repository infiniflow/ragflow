import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo, useEffect, useState, type ComponentProps } from 'react';
import { useForm, useFormContext } from 'react-hook-form';
import { z } from 'zod';
import { initialFXMacroDataValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import operations from './operations.json';

export const FXMacroDataFormPartialSchema = {
  operation: z.string(),
  arguments: z.record(z.string(), z.unknown()),
  use_credentials: z.boolean(),
  timeout: z.coerce.number().min(1).max(120),
};
const FormSchema = z.object(FXMacroDataFormPartialSchema);
const outputList = buildOutputList(initialFXMacroDataValues.outputs);
type Schema = {
  type?: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: unknown[];
  anyOf?: Schema[];
};
type OperationDefinition = {
  name: string;
  description: string;
  input_schema: { properties: Record<string, Schema>; required?: string[] };
};

function structuredText(value: unknown) {
  return value === undefined
    ? ''
    : typeof value === 'string'
      ? value
      : JSON.stringify(value);
}

function StructuredInput({
  value,
  onChange,
  onError,
  ...inputProps
}: Omit<ComponentProps<typeof Input>, 'value' | 'onChange' | 'onError'> & {
  value: unknown;
  onChange: (value: unknown) => void;
  onError: (message?: string) => void;
}) {
  const [text, setText] = useState(() => structuredText(value));
  useEffect(() => {
    setText(structuredText(value));
  }, [value]);
  return (
    <Input
      {...inputProps}
      value={text}
      onChange={(event) => {
        const text = event.target.value;
        setText(text);
        if (!text.trim()) {
          onChange(undefined);
          onError();
          return;
        }
        try {
          const parsed: unknown = JSON.parse(text);
          onChange(parsed);
          onError();
        } catch {
          // Persist the invalid replacement, so native schema validation blocks
          // execution instead of silently reusing the previous valid arguments.
          onChange(text);
          onError('Enter valid JSON for this structured field.');
        }
      }}
    />
  );
}

export function FXMacroDataWidgets({
  agentTool = false,
}: {
  agentTool?: boolean;
}) {
  const form = useFormContext();
  const selected = form.watch('operation');
  const operation = operations.find((item) => item.name === selected) as
    | OperationDefinition
    | undefined;
  const properties = (operation?.input_schema.properties ?? {}) as Record<
    string,
    Schema
  >;
  return (
    <>
      <a
        href="https://fxmacrodata.com/documentation/reference?utm_source=ragflow&utm_medium=integration&utm_campaign=open_source_integrations&utm_content=app"
        target="_blank"
        rel="noreferrer"
      >
        FXMacroData API reference
      </a>
      <FormField
        control={form.control}
        name="operation"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Operation</FormLabel>
            <FormControl>
              <RAGFlowSelect
                value={field.value}
                options={operations.map((item) => ({
                  value: item.name,
                  label: item.name,
                }))}
                onChange={(value) => {
                  field.onChange(value);
                  form.setValue(
                    'arguments',
                    value === 'release_calendar' ? { currency: 'usd' } : {},
                    { shouldDirty: true },
                  );
                }}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <p>{operation?.description}</p>
      <FormField
        control={form.control}
        name="use_credentials"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Data access</FormLabel>
            <FormControl>
              <RAGFlowSelect
                value={field.value ? 'authorized' : 'public'}
                options={[
                  { value: 'public', label: 'Public access (no key)' },
                  { value: 'authorized', label: 'Deployment credential' },
                ]}
                onChange={(value) => field.onChange(value === 'authorized')}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="timeout"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Request timeout (seconds)</FormLabel>
            <FormControl>
              <Input
                type="number"
                min={1}
                max={120}
                {...field}
                onChange={(event) => field.onChange(Number(event.target.value))}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      {agentTool ? (
        <p>
          The agent supplies the selected operation&apos;s typed arguments. Add
          additional FXMacroData tools for other operations.
        </p>
      ) : (
        Object.entries(properties).map(([name, schema]) => {
          const effective =
            schema.anyOf?.find((option) => option.type !== 'null') ?? schema;
          const nullable =
            schema.type === 'null' ||
            schema.anyOf?.some((option) => option.type === 'null');
          return (
            <FormField
              key={`${selected}:${name}`}
              control={form.control}
              name={`arguments.${name}`}
              render={({ field }) => (
                <FormItem>
                  <FormLabel tooltip={schema.description}>
                    {schema.title ?? name}
                    {operation?.input_schema.required?.includes(name)
                      ? ' *'
                      : ''}
                  </FormLabel>
                  {nullable && (
                    <label className="flex items-center gap-2 text-sm">
                      <Checkbox
                        aria-label={`Send null for ${name}`}
                        checked={field.value === null}
                        onCheckedChange={(checked) => {
                          if (checked === true) field.onChange(null);
                          else form.unregister(`arguments.${name}`);
                          form.clearErrors(`arguments.${name}`);
                        }}
                      />
                      Send null
                    </label>
                  )}
                  <FormControl>
                    {field.value === null && nullable ? (
                      <Input value="null" disabled />
                    ) : effective.type === 'boolean' ? (
                      <RAGFlowSelect
                        value={
                          field.value === undefined
                            ? 'omit'
                            : String(field.value)
                        }
                        options={[
                          { value: 'omit', label: 'Use API default' },
                          { value: 'true', label: 'True' },
                          { value: 'false', label: 'False' },
                        ]}
                        onChange={(value) => {
                          if (value === 'omit')
                            form.unregister(`arguments.${name}`);
                          else field.onChange(value === 'true');
                        }}
                      />
                    ) : effective.type === 'array' ||
                      effective.type === 'object' ? (
                      <StructuredInput
                        value={field.value}
                        onChange={(value) => {
                          if (value === undefined)
                            form.unregister(`arguments.${name}`);
                          else field.onChange(value);
                        }}
                        onError={(message) => {
                          if (message)
                            form.setError(`arguments.${name}`, { message });
                          else form.clearErrors(`arguments.${name}`);
                        }}
                      />
                    ) : (
                      <Input
                        value={field.value ?? ''}
                        type={
                          effective.type === 'integer' ||
                          effective.type === 'number'
                            ? 'number'
                            : 'text'
                        }
                        placeholder={schema.description}
                        onChange={(event) => {
                          const value = event.target.value;
                          if (value === '') {
                            form.unregister(`arguments.${name}`);
                            return;
                          }
                          if (
                            effective.type === 'integer' ||
                            effective.type === 'number'
                          )
                            field.onChange(Number(value));
                          else field.onChange(value);
                        }}
                      />
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          );
        })
      )}
    </>
  );
}

function FXMacroDataForm({ node }: INextOperatorForm) {
  const defaultValues = useFormValues(initialFXMacroDataValues, node);
  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });
  useWatchFormChange(node?.id, form);
  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <FXMacroDataWidgets />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList} />
      </div>
    </Form>
  );
}

export default memo(FXMacroDataForm);
