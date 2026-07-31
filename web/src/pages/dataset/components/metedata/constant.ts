import { TFunction } from 'i18next';
import { MetadataValueType } from './interface';

export enum MetadataType {
  Manage = 1,
  UpdateSingle = 2,
  Setting = 3,
  SingleFileSetting = 4,
}

export const MetadataDeleteMap = (
  t: TFunction<'translation', undefined>,
): Record<
  MetadataType,
  {
    title: string;
    warnFieldText: string;
    warnValueText: string;
    warnFieldName: string;
    warnValueName: string;
  }
> => {
  return {
    [MetadataType.Manage]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteManageFieldAllWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteManageValueAllWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldNameExists'),
      warnValueName: t('knowledgeDetails.metadata.valueExists'),
    },
    [MetadataType.Setting]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteSettingFieldWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteSettingValueWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldExists'),
      warnValueName: t('knowledgeDetails.metadata.valueExists'),
    },
    [MetadataType.UpdateSingle]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteManageFieldSingleWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteManageValueSingleWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldSingleNameExists'),
      warnValueName: t('knowledgeDetails.metadata.valueSingleExists'),
    },
    [MetadataType.SingleFileSetting]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteSettingFieldWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteSettingValueWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldExists'),
      warnValueName: t('knowledgeDetails.metadata.valueSingleExists'),
    },
  };
};

export const DEFAULT_VALUE_TYPE: MetadataValueType = 'string';
// const VALUE_TYPES_WITH_ENUM = new Set<MetadataValueType>(['enum']);
export const VALUE_TYPE_LABELS: Record<MetadataValueType, string> = {
  string: 'String',
  time: 'Time',
  number: 'Number',
  // bool: 'Bool',
  // enum: 'Enum',
  list: 'List',
  // int: 'Int',
  // float: 'Float',
};

export const metadataValueTypeEnum = Object.keys(VALUE_TYPE_LABELS).reduce(
  (acc, item) => {
    return { ...acc, [item]: item };
  },
  {} as Record<MetadataValueType, MetadataValueType>,
);

export const metadataValueTypeOptions = Object.entries(VALUE_TYPE_LABELS).map(
  ([value, label]) => ({ label, value }),
);

export const getMetadataValueTypeLabel = (value?: MetadataValueType) =>
  VALUE_TYPE_LABELS[value || DEFAULT_VALUE_TYPE] || VALUE_TYPE_LABELS.string;
