import { FieldValues, Path, PathValue, UseFormReturn } from 'react-hook-form';

export type PaginationProps = {
  current?: number;
  pageSize?: number;
  total?: number;
  showSizeChanger?: boolean;
  showQuickJumper?: boolean;
  pageSizeOptions?: number[];
  onChange?: (page: number, pageSize?: number) => void;
};

export type DefaultOptionType = {
  label: string | React.ReactNode;
  value: string | number;
  disabled?: boolean;
  children?: DefaultOptionType[];
};

export type UploadFile = {
  uid: string;
  name: string;
  status?: 'uploading' | 'done' | 'error' | 'removed';
  url?: string;
  thumbUrl?: string;
  response?: any;
  error?: any;
  size?: number;
  type?: string;
  lastModified?: number;
  percent?: number;
  originFileObj?: File;
};

export type TableRowSelection<T = any> = {
  selectedRowKeys?: React.Key[];
  onChange?: (selectedRowKeys: React.Key[], selectedRows: T[]) => void;
  getCheckboxProps?: (record: T) => {
    disabled?: boolean;
  };
};

export type FormInstance<TFieldValues extends FieldValues = FieldValues> = {
  getFieldValue: (name: string | string[]) => any;
  getFieldsValue: (names?: string[]) => Record<string, any>;
  setFieldValue: (name: string | string[], value: any) => void;
  setFieldsValue: (values: Record<string, any>) => void;
  resetFields: (fields?: string[]) => void;
  validateFields: (fields?: string[]) => Promise<any>;
  getFieldsError: (fields?: string[]) => Array<{
    name: string | string[];
    errors: string[];
  }>;
  getFieldError: (name: string | string[]) => string[];
  isFieldTouched: (name: string | string[]) => boolean;
  isFieldsTouched: (fields?: string[]) => boolean;
};

export type FormListFieldData = {
  name: number;
  key: number;
  isListField?: boolean;
  fieldKey?: number;
};

export function createFormInstance<
  TFieldValues extends FieldValues = FieldValues,
>(form: UseFormReturn<TFieldValues>): FormInstance<TFieldValues> {
  return {
    getFieldValue: (name) => {
      const path = Array.isArray(name) ? name.join('.') : name;
      return form.getValues(path as Path<TFieldValues>);
    },
    getFieldsValue: (names) => {
      if (names) {
        return names.reduce(
          (acc, name) => {
            acc[name] = form.getValues(name as Path<TFieldValues>);
            return acc;
          },
          {} as Record<string, any>,
        );
      }
      return form.getValues();
    },
    setFieldValue: (name, value) => {
      const path = Array.isArray(name) ? name.join('.') : name;
      form.setValue(
        path as Path<TFieldValues>,
        value as PathValue<TFieldValues, Path<TFieldValues>>,
      );
    },
    setFieldsValue: (values) => {
      Object.entries(values).forEach(([key, value]) => {
        form.setValue(
          key as Path<TFieldValues>,
          value as PathValue<TFieldValues, Path<TFieldValues>>,
        );
      });
    },
    resetFields: (fields) => {
      if (fields) {
        fields.forEach((field) => form.resetField(field as Path<TFieldValues>));
      } else {
        form.reset();
      }
    },
    validateFields: async (fields) => {
      return form
        .trigger(fields as Path<TFieldValues>[])
        .then(() => form.getValues());
    },
    getFieldsError: (fields) => {
      const errors = form.formState.errors;
      return Object.entries(errors).map(([name, error]) => ({
        name,
        errors: error ? [String(error.message)] : [],
      }));
    },
    getFieldError: (name) => {
      const path = Array.isArray(name) ? name.join('.') : name;
      const error = form.formState.errors[path as Path<TFieldValues>];
      return error ? [String(error.message)] : [];
    },
    isFieldTouched: (name) => {
      const path = Array.isArray(name) ? name.join('.') : name;
      return form.formState.touchedFields[path as Path<TFieldValues>] ?? false;
    },
    isFieldsTouched: (fields) => {
      if (fields) {
        return fields.some(
          (field) => form.formState.touchedFields[field as Path<TFieldValues>],
        );
      }
      return Object.keys(form.formState.touchedFields).length > 0;
    },
  };
}
