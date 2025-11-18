import { zodResolver } from '@hookform/resolvers/zod';
import { forwardRef, useEffect, useImperativeHandle, useMemo } from 'react';
import {
  DefaultValues,
  FieldValues,
  SubmitHandler,
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

// Field type enumeration
export enum FormFieldType {
  Text = 'text',
  Email = 'email',
  Password = 'password',
  Number = 'number',
  Textarea = 'textarea',
  Select = 'select',
  Checkbox = 'checkbox',
  Tag = 'tag',
}

// Field configuration interface
export interface FormFieldConfig {
  name: string;
  label: string;
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
  render?: (fieldProps: any) => React.ReactNode;
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
}

// Component props interface
interface DynamicFormProps<T extends FieldValues> {
  fields: FormFieldConfig[];
  onSubmit: SubmitHandler<T>;
  className?: string;
  children?: React.ReactNode;
  defaultValues?: DefaultValues<T>;
  onFieldUpdate?: (
    fieldName: string,
    updatedField: Partial<FormFieldConfig>,
  ) => void;
}

// Form ref interface
export interface DynamicFormRef {
  submit: () => void;
  getValues: () => any;
  reset: (values?: any) => void;
  watch: (field: string, callback: (value: any) => void) => () => void;
  updateFieldType: (fieldName: string, newType: FormFieldType) => void;
}

// Generate Zod validation schema based on field configurations
const generateSchema = (fields: FormFieldConfig[]): ZodSchema<any> => {
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
      if (field.type === FormFieldType.Checkbox) {
        fieldSchema = (fieldSchema as z.ZodBoolean).refine(
          (val) => val === true,
          {
            message: `${field.label} is required`,
          },
        );
      } else if (field.type === FormFieldType.Tag) {
        fieldSchema = (fieldSchema as z.ZodArray<z.ZodString>).min(1, {
          message: `${field.label} is required`,
        });
      } else {
        fieldSchema = (fieldSchema as z.ZodString).min(1, {
          message: `${field.label} is required`,
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
      } else if (field.type === FormFieldType.Checkbox) {
        current[lastKey] = false;
      } else if (field.type === FormFieldType.Tag) {
        current[lastKey] = [];
      } else {
        current[lastKey] = '';
      }
    } else {
      if (field.defaultValue !== undefined) {
        defaultValues[field.name] = field.defaultValue;
      } else if (field.type === FormFieldType.Checkbox) {
        defaultValues[field.name] = false;
      } else if (field.type === FormFieldType.Tag) {
        defaultValues[field.name] = [];
      } else {
        defaultValues[field.name] = '';
      }
    }
  });

  return defaultValues as DefaultValues<T>;
};

// Dynamic form component
const DynamicForm = {
  Root: forwardRef(
    <T extends FieldValues>(
      {
        fields,
        onSubmit,
        className = '',
        children,
        defaultValues: formDefaultValues = {} as DefaultValues<T>,
        onFieldUpdate,
      }: DynamicFormProps<T>,
      ref: React.Ref<any>,
    ) => {
      // Generate validation schema and default values
      const schema = useMemo(() => generateSchema(fields), [fields]);

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
          const zodResult = await zodResolver(schema)(data, context, options);

          let combinedErrors = { ...zodResult.errors };

          const fieldErrors: Record<string, { type: string; message: string }> =
            {};
          for (const field of fields) {
            if (field.customValidate && data[field.name] !== undefined) {
              try {
                const result = await field.customValidate(
                  data[field.name],
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

      // Expose form methods via ref
      useImperativeHandle(ref, () => ({
        submit: () => form.handleSubmit(onSubmit)(),
        getValues: () => form.getValues(),
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
          setTimeout(() => {
            if (onFieldUpdate) {
              onFieldUpdate(fieldName, updatedField);
            } else {
              console.warn(
                'onFieldUpdate prop is not provided. Cannot update field type.',
              );
            }
          }, 0);
        },
      }));

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

      // Render form fields
      const renderField = (field: FormFieldConfig) => {
        if (field.render) {
          return (
            <RAGFlowFormItem
              name={field.name}
              label={field.label}
              required={field.required}
              horizontal={field.horizontal}
              tooltip={field.tooltip}
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
          case FormFieldType.Textarea:
            return (
              <RAGFlowFormItem
                name={field.name}
                label={field.label}
                required={field.required}
                horizontal={field.horizontal}
                tooltip={field.tooltip}
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
                      // className="resize-none"
                    />
                  );
                }}
              </RAGFlowFormItem>
            );

          case FormFieldType.Select:
            return (
              <RAGFlowFormItem
                name={field.name}
                label={field.label}
                required={field.required}
                horizontal={field.horizontal}
                tooltip={field.tooltip}
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
                      'flex-row items-center space-x-3 space-y-0':
                        !field.horizontal,
                    })}
                  >
                    {field.label && !field.horizontal && (
                      <div className="space-y-1 leading-none">
                        <FormLabel
                          className="font-normal"
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
                          className="font-normal"
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
                        />
                      </div>
                    </FormControl>

                    <FormMessage />
                  </FormItem>
                )}
              />
            );

          case FormFieldType.Tag:
            return (
              <RAGFlowFormItem
                name={field.name}
                label={field.label}
                required={field.required}
                horizontal={field.horizontal}
                tooltip={field.tooltip}
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
                      <EditTag {...finalFieldProps}></EditTag>
                    </div>
                  );
                }}
              </RAGFlowFormItem>
            );

          default:
            return (
              <RAGFlowFormItem
                name={field.name}
                label={field.label}
                required={field.required}
                horizontal={field.horizontal}
                tooltip={field.tooltip}
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
                      />
                    </div>
                  );
                }}
              </RAGFlowFormItem>
            );
        }
      };

      // Watch all form values to re-render when they change (for shouldRender checks)
      const formValues = form.watch();

      return (
        <Form {...form}>
          <form
            className={`space-y-6 ${className}`}
            onSubmit={(e) => {
              e.preventDefault();
              form.handleSubmit(onSubmit)(e);
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
                    {renderField(field)}
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
    submitLoading: boolean;
    buttonText?: string;
    submitFunc?: (values: FieldValues) => void;
  }) => {
    const form = useFormContext();
    return (
      <button
        type="button"
        disabled={submitLoading}
        onClick={() => {
          console.log('form submit');
          (async () => {
            console.log('form submit2');
            try {
              let beValid = await form.formControl.trigger();
              console.log('form valid', beValid, form, form.formControl);
              if (beValid) {
                form.handleSubmit(async (values) => {
                  console.log('form values', values);
                  submitFunc?.(values);
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
        className="px-2 py-1 border border-input rounded-md hover:bg-muted"
      >
        {cancelText ?? t('modal.cancelText')}
      </button>
    );
  },
};

export { DynamicForm };
