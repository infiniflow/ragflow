import { ReactNode } from 'react';
import { MetadataType } from './constant';
export type IMetaDataReturnType = Record<
  string,
  | { type: string; values: Array<Array<string | number>> }
  | Array<Array<string | number>>
>;
export type IMetaDataReturnJSONType = Record<
  string,
  Array<string | number> | string
>;

export interface IMetaDataReturnJSONSettingItem {
  key: string;
  description?: string;
  enum?: string[];
}
export interface IMetaDataJsonSchemaProperty {
  type?: string;
  description?: string;
  enum?: string[];
  items?: {
    type?: string;
    enum?: string[];
  };
  format?: string;
}
export interface IMetaDataJsonSchema {
  type?: 'object';
  properties?: Record<string, IMetaDataJsonSchemaProperty>;
  additionalProperties?: boolean;
}
export type IMetaDataReturnJSONSettings =
  | IMetaDataJsonSchema
  | Array<IMetaDataReturnJSONSettingItem>;

export type MetadataValueType =
  | 'string'
  | 'list'
  // | 'bool'
  // | 'enum'
  | 'time'
  | 'number';

export type IMetaDataTableData = {
  field: string;
  description: string;
  restrictDefinedValues?: boolean;
  values: string[];
  valueType?: MetadataValueType;
};

export type IBuiltInMetadataItem = {
  key: string;
  type: MetadataValueType;
};

export type IManageModalProps = {
  documentIds?: string[];
  title: ReactNode;
  isShowDescription?: boolean;
  isDeleteSingleValue?: boolean;
  visible: boolean;
  hideModal: () => void;
  tableData?: IMetaDataTableData[];
  isCanAdd: boolean;
  type: MetadataType;
  otherData?: Record<string, any>;
  isEditField?: boolean;
  isAddValue?: boolean;
  isShowValueSwitch?: boolean;
  isVerticalShowValue?: boolean;
  builtInMetadata?: IBuiltInMetadataItem[];
  success?: (data: any) => void;
  secondTitle?: ReactNode;
};

export interface IManageValuesProps {
  title: ReactNode;
  existsKeys: string[];
  visible: boolean;
  isEditField?: boolean;
  isAddValue?: boolean;
  isShowDescription?: boolean;
  isShowValueSwitch?: boolean;
  isShowType?: boolean;
  isVerticalShowValue?: boolean;
  isAddValueMode?: boolean;
  data: IMetaDataTableData;
  type: MetadataType;
  hideModal: () => void;
  onSave: (data: IMetaDataTableData) => void;
  addUpdateValue: (
    key: string,
    originalValue: string | undefined,
    newValue: string | string[],
    type?: MetadataValueType,
  ) => void;
  addDeleteValue: (key: string, value: string) => void;
}

export interface DeleteOperation {
  key: string;
  value?: string;
}

export interface UpdateOperation {
  key: string;
  match: string;
  value: string | string[];
  valueType?: MetadataValueType;
}

export interface MetadataOperations {
  deletes: DeleteOperation[];
  updates: UpdateOperation[];
}
export interface ShowManageMetadataModalOptions {
  title?: ReactNode | string;
}
export type ShowManageMetadataModalProps = Partial<IManageModalProps> & {
  metadata?: IMetaDataTableData[];
  isCanAdd: boolean;
  type: MetadataType;
  record?: Record<string, any>;
  builtInMetadata?: IBuiltInMetadataItem[];
  options?: ShowManageMetadataModalOptions;
  title?: ReactNode | string;
  isDeleteSingleValue?: boolean;
  documentIds?: string[];
};
