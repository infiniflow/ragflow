import { FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const YandexDiskConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldAccessToken'),
    name: 'config.credentials.oauth_token',
    type: FormFieldType.Password,
    required: true,
    tooltip: t('setting.yandexDiskTokenTip'),
  },
  {
    label: t('setting.dataSourceFieldFolderPathOptional'),
    name: 'config.path',
    type: FormFieldType.Text,
    required: false,
    defaultValue: '/',
    placeholder: '/',
    tooltip: t('setting.yandexDiskPathTip'),
  },
];
