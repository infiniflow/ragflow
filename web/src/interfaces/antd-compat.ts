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

export type FormInstance = {
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
