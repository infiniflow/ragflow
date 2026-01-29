import { zodResolver } from '@hookform/resolvers/zod';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useState,
} from 'react';
import {
  ControllerRenderProps,
  DefaultValues,
  FieldValues,
  SubmitHandler,
  UseFormTrigger,
  useForm,
  useFormContext,
} from 'react-hook-form';
import { ZodSchema, z } from 'zod';

import EditTag from '@/components/edit-tag';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { Loader } from 'lucide-react';
import { MultiSelect, MultiSelectOptionType } from './ui/multi-select';
import { Segmented } from './ui/segmented';
import { Switch } from './ui/switch';

const getNestedValue = (obj: any, path: string) => {
  return path.split('.').reduce((current, key) => {
    return current && current[key] !== undefined ? current[key] : undefined;
  }, obj);
};

/**
 * Properties of this field will be treated as static attributes and will be filtered out during form submission.
 */
export const FilterFormField = 'RAG_DY_STATIC';

// Field type enumeration
export enum FormFieldType {
  Text = 'text',
  Email = 'email',
  Password = 'password',
  Number = 'number',
  Textarea = 'textarea',
  Select = 'select',
  MultiSelect = 'multi-select',
  Checkbox = 'checkbox',
  Switch = 'switch',
  Tag = 'tag',
  Segmented = 'segmented',
  Custom = 'custom',
}

// Field configuration interface
export interface FormFieldConfig {
  name: string;
  label: string;
  hideLabel?: boolean;
  type: FormFieldType;
  hidden?: boolean;
  required?: boolean;
  placeholder?: string;
  options?: { label: string; value: string }[];
  defaultValue?: any;
  validation?: {
    pattern?: RegExp;
    minLength?: number;
    maxLength?: number;
    min?: number;
    max?: number;
    message?: string;
  };
  render?: (fieldProps: ControllerRenderProps) => React.ReactNode;
  horizontal?: boolean;
  onChange?: (value: any) => void;
  tooltip?: React.ReactNode;
  customValidate?: (
    value: any,
    formValues: any,
  ) => string | boolean | Promise<string | boolean>;
  dependencies?: string[];
  schema?: ZodSchema;
  shouldRender?: (formValues: any) => boolean;
  labelClassName?: string;
  className?: string;
  disabled?: boolean;
}

// Component props interface
interface DynamicFormProps<T extends FieldValues> {
  fields: FormFieldConfig[];
  onSubmit: SubmitHandler<T>;
  className?: string;
  children?: React.ReactNode;
  defaultValues?: DefaultValues<T>;
  // onFieldUpdate?: (
  //   fieldName: string,
  //   updatedField: Partial<FormFieldConfig>,
  // ) => void;
  labelClassName?: string;
}

// Form ref interface
export interface DynamicFormRef {
  submit: () => void;
  getValues: (name?: string) => any;
  reset: (values?: any) => void;
  trigger: UseFormTrigger<any>;
  watch: (field: string, callback: (value: any) => void) => () => void;
  updateFieldType: (fieldName: string, newType: FormFieldType) => void;
  onFieldUpdate: (
    fieldName: string,
    newFieldProperties: Partial<FormFieldConfig>,
  ) => void;
}

// Generate Zod validation schema based on field configurations
export const generateSchema = (fields: FormFieldConfig[]): ZodSchema<any> => {
  const schema: Record<string, ZodSchema> = {};
  const nestedSchemas: Record<string, Record<string, ZodSchema>> = {};

  fields.forEach((field) => {
    let fieldSchema: ZodSchema;

    // Create base validation schema based on field type
    if (field.schema) {
      fieldSchema = field.schema;
    } else {
      switch (field.type) {
        case FormFieldType.Email:
          fieldSchema = z.string().email('Please enter a valid email address');
          break;
        case FormFieldType.MultiSelect:
          fieldSchema = z.array(z.string()).optional();
          if (field.required) {
            fieldSchema = z.array(z.string()).min(1, {
              message: `${field.label} is required`,
            });
          }
          break;
        case FormFieldType.Segmented:
          fieldSchema = z.string();
          break;
        case FormFieldType.Number:
          fieldSchema = z.coerce.number();
          if (field.validation?.min !== undefined) {
            fieldSchema = (fieldSchema as z.ZodNumber).min(
              field.validation.min,
              field.validation.message ||
                `Value cannot be less than ${field.validation.min}`,
            );
          }
          if (field.validation?.max !== undefined) {
            fieldSchema = (fieldSchema as z.ZodNumber).max(
              field.validation.max,
              field.validation.message ||
                `Value cannot be greater than ${field.validation.max}`,
            );
          }
          break;
        case FormFieldType.Checkbox:
        case FormFieldType.Switch:
          fieldSchema = z.boolean();
          break;
        case FormFieldType.Tag:
          fieldSchema = z.array(z.string());
          break;
        default:
          fieldSchema = z.string();
          break;
      }
    }

    // Handle required fields
    if (field.required) {
      const requiredMessage =
        field.validation?.message || `${field.label} is required`;

      if (field.type === FormFieldType.Checkbox) {
        fieldSchema = (fieldSchema as z.ZodBoolean).refine(
          (val) => val === true,
          {
            message: requiredMessage,
          },
        );
      } else if (field.type === FormFieldType.Tag) {
        fieldSchema = (fieldSchema as z.ZodArray<z.ZodString>).min(1, {
          message: requiredMessage,
        });
      } else {
        fieldSchema = (fieldSchema as z.ZodString).min(1, {
          message: requiredMessage,
        });
      }
    }

    if (!field.required) {
      fieldSchema = fieldSchema.optional();
    }

    // Handle other validation rules
    if (
      field.type !== FormFieldType.Number &&
      field.type !== FormFieldType.Checkbox &&
      field.type !== FormFieldType.Switch &&
      field.type !== FormFieldType.Custom &&
      field.type !== FormFieldType.Tag &&
      field.required
    ) {
      fieldSchema = fieldSchema as z.ZodString;

      if (field.validation?.minLength !== undefined) {
        fieldSchema = (fieldSchema as z.ZodString).min(
          field.validation.minLength,
          field.validation.message ||
            `Enter at least ${field.validation.minLength} characters`,
        );
      }

      if (field.validation?.maxLength !== undefined) {
        fieldSchema = (fieldSchema as z.ZodString).max(
          field.validation.maxLength,
          field.validation.message ||
            `Enter up to ${field.validation.maxLength} characters`,
        );
      }

      if (field.validation?.pattern) {
        fieldSchema = (fieldSchema as z.ZodString).regex(
          field.validation.pattern,
          field.validation.message || 'Invalid input format',
        );
      }
    }

    if (field.name.includes('.')) {
      const keys = field.name.split('.');
      const firstKey = keys[0];

      if (!nestedSchemas[firstKey]) {
        nestedSchemas[firstKey] = {};
      }

      let currentSchema = nestedSchemas[firstKey];
      for (let i = 1; i < keys.length - 1; i++) {
        const key = keys[i];
        if (!currentSchema[key]) {
          currentSchema[key] = {};
        }
        currentSchema = currentSchema[key];
      }

      const lastKey = keys[keys.length - 1];
      currentSchema[lastKey] = fieldSchema;
    } else {
      schema[field.name] = fieldSchema;
    }
  });

  Object.keys(nestedSchemas).forEach((key) => {
    const buildNestedSchema = (obj: Record<string, any>): ZodSchema => {
      const nestedSchema: Record<string, ZodSchema> = {};
      Object.keys(obj).forEach((subKey) => {
        if (
          typeof obj[subKey] === 'object' &&
          !(obj[subKey] instanceof z.ZodType)
        ) {
          nestedSchema[subKey] = buildNestedSchema(obj[subKey]);
        } else {
          nestedSchema[subKey] = obj[subKey];
        }
      });
      return z.object(nestedSchema);
    };

    schema[key] = buildNestedSchema(nestedSchemas[key]);
  });
  return z.object(schema);
};

// Generate default values based on field configurations
const generateDefaultValues = <T extends FieldValues>(
  fields: FormFieldConfig[],
): DefaultValues<T> => {
  const defaultValues: Record<string, any> = {};

  fields.forEach((field) => {
    if (field.name.includes('.')) {
      const keys = field.name.split('.');
      let current = defaultValues;

      for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        if (!current[key]) {
          current[key] = {};
        }
        current = current[key];
      }

      const lastKey = keys[keys.length - 1];
      if (field.defaultValue !== undefined) {
        current[lastKey] = field.defaultValue;
      } else if (
        field.type === FormFieldType.Checkbox ||
        field.type === FormFieldType.Switch
      ) {
        current[lastKey] = false;
      } else if (field.type === FormFieldType.Tag) {
        current[lastKey] = [];
      } else {
        current[lastKey] = '';
      }
    } else {
      if (field.defaultValue !== undefined) {
        defaultValues[field.name] = field.defaultValue;
      } else if (
        field.type === FormFieldType.Checkbox ||
        field.type === FormFieldType.Switch
      ) {
        defaultValues[field.name] = false;
      } else if (
        field.type === FormFieldType.Tag ||
        field.type === FormFieldType.MultiSelect
      ) {
        defaultValues[field.name] = [];
      } else {
        defaultValues[field.name] = '';
      }
    }
  });

  return defaultValues as DefaultValues<T>;
};
// Render form fields
export const RenderField = ({
  field,
  labelClassName,
}: {
  field: FormFieldConfig;
  labelClassName?: string;
}) => {
  const form = useFormContext();
  if (field.render) {
    if (field.type === FormFieldType.Custom && field.hideLabel) {
      return <div className="w-full">{field.render({})}</div>;
    }
    return (
      <RAGFlowFormItem
        {...field}
        labelClassName={labelClassName || field.labelClassName}
      >
        {(fieldProps) => {
          const finalFieldProps = field.onChange
            ? {
                ...fieldProps,
                onChange: (e: any) => {
                  fieldProps.onChange(e);
                  field.onChange?.(e.target?.value ?? e);
                },
              }
            : fieldProps;
          return field.render?.(finalFieldProps);
        }}
      </RAGFlowFormItem>
    );
  }
  switch (field.type) {
    case FormFieldType.Segmented:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (value: any) => {
                    fieldProps.onChange(value);
                    field.onChange?.(value);
                  },
                }
              : fieldProps;
            return (
              <Segmented
                {...finalFieldProps}
                options={field.options || []}
                className="w-full"
                itemClassName="flex-1 justify-center"
                disabled={field.disabled}
              />
            );
          }}
        </RAGFlowFormItem>
      );
    case FormFieldType.Textarea:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (e: any) => {
                    fieldProps.onChange(e);
                    field.onChange?.(e.target.value);
                  },
                }
              : fieldProps;
            return (
              <Textarea
                {...finalFieldProps}
                placeholder={field.placeholder}
                disabled={field.disabled}
                // className="resize-none"
              />
            );
          }}
        </RAGFlowFormItem>
      );

    case FormFieldType.Select:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (value: string) => {
                    console.log('select value', value);
                    if (fieldProps.onChange) {
                      fieldProps.onChange(value);
                    }
                    field.onChange?.(value);
                  },
                }
              : fieldProps;
            return (
              <SelectWithSearch
                triggerClassName="!shrink"
                {...finalFieldProps}
                options={field.options}
                disabled={field.disabled}
              />
            );
          }}
        </RAGFlowFormItem>
      );

    case FormFieldType.MultiSelect:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            console.log('multi select value', fieldProps);
            const finalFieldProps = {
              ...fieldProps,
              onValueChange: (value: string[]) => {
                if (fieldProps.onChange) {
                  fieldProps.onChange(value);
                }
                field.onChange?.(value);
              },
            };
            return (
              <MultiSelect
                variant="inverted"
                maxCount={100}
                {...finalFieldProps}
                // onValueChange={(data) => {
                //   console.log(data);
                //   field.onChange?.(data);
                // }}
                options={field.options as MultiSelectOptionType[]}
                disabled={field.disabled}
              />
            );
          }}
        </RAGFlowFormItem>
      );

    case FormFieldType.Checkbox:
      return (
        <FormField
          control={form.control}
          name={field.name as any}
          render={({ field: formField }) => (
            <FormItem
              className={cn('flex items-center w-full', {
                'flex-row items-center space-x-3 space-y-0': !field.horizontal,
              })}
            >
              {field.label && !field.horizontal && (
                <div className="space-y-1 leading-none">
                  <FormLabel
                    className={cn(
                      'font-medium',
                      labelClassName || field.labelClassName,
                    )}
                    tooltip={field.tooltip}
                  >
                    {field.label}{' '}
                    {field.required && (
                      <span className="text-destructive">*</span>
                    )}
                  </FormLabel>
                </div>
              )}
              {field.label && field.horizontal && (
                <div className="space-y-1 leading-none w-1/4">
                  <FormLabel
                    className={cn(
                      'font-medium',
                      labelClassName || field.labelClassName,
                    )}
                    tooltip={field.tooltip}
                  >
                    {field.label}{' '}
                    {field.required && (
                      <span className="text-destructive">*</span>
                    )}
                  </FormLabel>
                </div>
              )}
              <FormControl>
                <div className={cn({ 'w-full': field.horizontal })}>
                  <Checkbox
                    checked={formField.value}
                    onCheckedChange={(checked) => {
                      formField.onChange(checked);
                      field.onChange?.(checked);
                    }}
                    disabled={field.disabled}
                  />
                </div>
              </FormControl>

              <FormMessage />
            </FormItem>
          )}
        />
      );
    case FormFieldType.Switch:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (checked: boolean) => {
                    fieldProps.onChange(checked);
                    field.onChange?.(checked);
                  },
                }
              : fieldProps;
            return (
              <Switch
                checked={finalFieldProps.value as boolean}
                onCheckedChange={(checked) => finalFieldProps.onChange(checked)}
                disabled={field.disabled}
              />
            );
          }}
        </RAGFlowFormItem>
      );

    case FormFieldType.Tag:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (value: string[]) => {
                    fieldProps.onChange(value);
                    field.onChange?.(value);
                  },
                }
              : fieldProps;
            return (
              //   <TagInput {...fieldProps} placeholder={field.placeholder} />
              <div className="w-full">
                <EditTag
                  {...finalFieldProps}
                  disabled={field.disabled}
                ></EditTag>
              </div>
            );
          }}
        </RAGFlowFormItem>
      );

    default:
      return (
        <RAGFlowFormItem
          {...field}
          labelClassName={labelClassName || field.labelClassName}
        >
          {(fieldProps) => {
            const finalFieldProps = field.onChange
              ? {
                  ...fieldProps,
                  onChange: (e: any) => {
                    fieldProps.onChange(e);
                    field.onChange?.(e.target.value);
                  },
                }
              : fieldProps;
            return (
              <div className="w-full">
                <Input
                  {...finalFieldProps}
                  type={field.type}
                  placeholder={field.placeholder}
                  disabled={field.disabled}
                />
              </div>
            );
          }}
        </RAGFlowFormItem>
      );
  }
};

// Dynamic form component
const DynamicForm = {
  Root: forwardRef(
    <T extends FieldValues>(
      {
        fields: originFields,
        onSubmit,
        className = '',
        children,
        defaultValues: formDefaultValues = {} as DefaultValues<T>,
        // onFieldUpdate,
        labelClassName,
      }: DynamicFormProps<T>,
      ref: React.Ref<any>,
    ) => {
      // Generate validation schema and default values
      const [fields, setFields] = useState(originFields);
      useMemo(() => {
        setFields(originFields);
      }, [originFields]);

      const defaultValues = useMemo(() => {
        const value = {
          ...generateDefaultValues(fields),
          ...formDefaultValues,
        };
        return value;
      }, [fields, formDefaultValues]);

      // Initialize form
      const form = useForm<T>({
        resolver: async (data, context, options) => {
          // Filter out fields that should not render
          const activeFields = fields.filter(
            (field) => !field.shouldRender || field.shouldRender(data),
          );

          const activeSchema = generateSchema(activeFields);
          const zodResult = await zodResolver(activeSchema)(
            data,
            context,
            options,
          );

          let combinedErrors = { ...zodResult.errors };

          const fieldErrors: Record<string, { type: string; message: string }> =
            {};
          for (const field of fields) {
            if (
              field.customValidate &&
              getNestedValue(data, field.name) !== undefined &&
              (!field.shouldRender || field.shouldRender(data))
            ) {
              try {
                const result = await field.customValidate(
                  getNestedValue(data, field.name),
                  data,
                );
                if (typeof result === 'string') {
                  fieldErrors[field.name] = {
                    type: 'custom',
                    message: result,
                  };
                } else if (result === false) {
                  fieldErrors[field.name] = {
                    type: 'custom',
                    message:
                      field.validation?.message || `${field.label} is invalid`,
                  };
                }
              } catch (error) {
                fieldErrors[field.name] = {
                  type: 'custom',
                  message:
                    error instanceof Error
                      ? error.message
                      : 'Validation failed',
                };
              }
            }
          }

          combinedErrors = {
            ...combinedErrors,
            ...fieldErrors,
          } as any;

          for (const key in combinedErrors) {
            if (Array.isArray(combinedErrors[key])) {
              combinedErrors[key] = combinedErrors[key][0];
            }
          }
          console.log('combinedErrors', combinedErrors);
          return {
            values: Object.keys(combinedErrors).length ? {} : data,
            errors: combinedErrors,
          } as any;
        },
        defaultValues,
      });

      useEffect(() => {
        const dependencyMap: Record<string, string[]> = {};

        fields.forEach((field) => {
          if (field.dependencies && field.dependencies.length > 0) {
            field.dependencies.forEach((dep) => {
              if (!dependencyMap[dep]) {
                dependencyMap[dep] = [];
              }
              dependencyMap[dep].push(field.name);
            });
          }
        });

        const subscriptions = Object.keys(dependencyMap).map((depField) => {
          return form.watch((values: any, { name }) => {
            if (name === depField && dependencyMap[depField]) {
              dependencyMap[depField].forEach((dependentField) => {
                form.trigger(dependentField as any);
              });
            }
          });
        });

        return () => {
          subscriptions.forEach((sub) => {
            if (sub.unsubscribe) {
              sub.unsubscribe();
            }
          });
        };
      }, [fields, form]);

      const filterActiveValues = useCallback(
        (allValues: any) => {
          const filteredValues: any = {};

          fields.forEach((field) => {
            if (
              !field.shouldRender ||
              (field.shouldRender(allValues) &&
                field.name?.indexOf(FilterFormField) < 0)
            ) {
              const keys = field.name.split('.');
              let current = allValues;
              let exists = true;

              for (const key of keys) {
                if (current && current[key] !== undefined) {
                  current = current[key];
                } else {
                  exists = false;
                  break;
                }
              }

              if (exists) {
                let target = filteredValues;
                for (let i = 0; i < keys.length - 1; i++) {
                  const key = keys[i];
                  if (!target[key]) {
                    target[key] = {};
                  }
                  target = target[key];
                }
                target[keys[keys.length - 1]] = getNestedValue(
                  allValues,
                  field.name,
                );
              }
            }
          });

          return filteredValues;
        },
        [fields],
      );

      // Expose form methods via ref
      useImperativeHandle(
        ref,
        () => ({
          form: form,
          submit: () => {
            form.handleSubmit((values) => {
              const filteredValues = filterActiveValues(values);
              onSubmit(filteredValues);
            })();
          },
          getValues: form.getValues,
          reset: (values?: T) => {
            if (values) {
              form.reset(values);
            } else {
              form.reset();
            }
          },
          setError: form.setError,
          clearErrors: form.clearErrors,
          trigger: form.trigger,
          watch: (field: string, callback: (value: any) => void) => {
            const { unsubscribe } = form.watch((values: any) => {
              if (values && values[field] !== undefined) {
                callback(values[field]);
              }
            });
            return unsubscribe;
          },

          onFieldUpdate: (
            fieldName: string,
            updatedField: Partial<FormFieldConfig>,
          ) => {
            setFields((prevFields: any) =>
              prevFields.map((field: any) =>
                field.name === fieldName
                  ? { ...field, ...updatedField }
                  : field,
              ),
            );
            // setTimeout(() => {
            //   if (onFieldUpdate) {
            //     onFieldUpdate(fieldName, updatedField);
            //   } else {
            //     console.warn(
            //       'onFieldUpdate prop is not provided. Cannot update field type.',
            //     );
            //   }
            // }, 0);
          },
        }),
        [form, onSubmit, filterActiveValues],
      );
      (form as any).filterActiveValues = filterActiveValues;
      useEffect(() => {
        if (formDefaultValues && Object.keys(formDefaultValues).length > 0) {
          form.reset({
            ...generateDefaultValues(fields),
            ...formDefaultValues,
          });
        }
      }, [form, formDefaultValues, fields]);

      // Submit handler
      //   const handleSubmit = form.handleSubmit(onSubmit);

      // Watch all form values to re-render when they change (for shouldRender checks)
      const formValues = form.watch();

      return (
        <Form {...form}>
          <form
            className={`space-y-6 ${className}`}
            onSubmit={(e) => {
              e.preventDefault();
              form.handleSubmit((values) => {
                const filteredValues = filterActiveValues(values);
                onSubmit(filteredValues);
              })(e);
            }}
          >
            <>
              {fields.map((field) => {
                const shouldShow = field.shouldRender
                  ? field.shouldRender(formValues)
                  : true;
                return (
                  <div
                    key={field.name}
                    className={cn({ hidden: field.hidden || !shouldShow })}
                  >
                    <RenderField
                      field={field}
                      labelClassName={labelClassName}
                    />
                  </div>
                );
              })}
              {children}
            </>
          </form>
        </Form>
      );
    },
  ) as <T extends FieldValues>(
    props: DynamicFormProps<T> & { ref?: React.Ref<DynamicFormRef> },
  ) => React.ReactElement,
  SavingButton: ({
    submitLoading,
    buttonText,
    submitFunc,
  }: {
    submitLoading?: boolean;
    buttonText?: string;
    submitFunc?: (values: FieldValues) => void;
  }) => {
    const form = useFormContext();
    return (
      <button
        type="button"
        disabled={submitLoading}
        onClick={() => {
          (async () => {
            try {
              let beValid = await form.trigger();
              console.log('form valid', beValid, form);
              // if (beValid) {
              //   form.handleSubmit(async (values) => {
              //     console.log('form values', values);
              //     submitFunc?.(values);
              //   })();
              // }

              if (beValid && submitFunc) {
                form.handleSubmit(async (values) => {
                  const filteredValues = (form as any).filterActiveValues
                    ? (form as any).filterActiveValues(values)
                    : values;
                  console.log(
                    'filtered form values in saving button',
                    filteredValues,
                  );
                  submitFunc(filteredValues);
                })();
              }
            } catch (e) {
              console.error(e);
            } finally {
              console.log('form submit3');
            }
          })();
        }}
        className={cn(
          'px-2 py-1 bg-primary text-primary-foreground rounded-md hover:bg-primary/90',
        )}
      >
        {submitLoading && (
          <Loader className="inline-block mr-2 h-4 w-4 animate-spin" />
        )}
        {buttonText ?? t('modal.okText')}
      </button>
    );
  },

  CancelButton: ({
    handleCancel,
    cancelText,
  }: {
    handleCancel: () => void;
    cancelText?: string;
  }) => {
    return (
      <button
        type="button"
        onClick={() => handleCancel()}
        className="px-2 py-1 border border-border-button rounded-md text-text-secondary hover:bg-bg-card hover:text-primary"
      >
        {cancelText ?? t('modal.cancelText')}
      </button>
    );
  },
};

DynamicForm.Root.displayName = 'DynamicFormRoot';

export { DynamicForm };
