import { ReactNode } from 'react';
import { MetadataType } from './hook';
export type IMetaDataReturnType = Record<string, Array<Array<string | number>>>;
export type IMetaDataReturnJSONType = Record<
  string,
  Array<string | number> | string
>;

export interface IMetaDataReturnJSONSettingItem {
  key: string;
  description?: string;
  enum?: string[];
}
export type IMetaDataReturnJSONSettings = Array<IMetaDataReturnJSONSettingItem>;

export type IMetaDataTableData = {
  field: string;
  description: string;
  restrictDefinedValues?: boolean;
  values: string[];
};

export type IManageModalProps = {
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
  success?: (data: any) => void;
};

export interface IManageValuesProps {
  title: ReactNode;
  visible: boolean;
  isEditField?: boolean;
  isAddValue?: boolean;
  isShowDescription?: boolean;
  isShowValueSwitch?: boolean;
  isVerticalShowValue?: boolean;
  data: IMetaDataTableData;
  hideModal: () => void;
  onSave: (data: IMetaDataTableData) => void;
  addUpdateValue: (key: string, value: string | string[]) => void;
  addDeleteValue: (key: string, value: string) => void;
}

interface DeleteOperation {
  key: string;
  value?: string;
}

interface UpdateOperation {
  key: string;
  value: string | string[];
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
  options?: ShowManageMetadataModalOptions;
  title?: ReactNode | string;
  isDeleteSingleValue?: boolean;
};
