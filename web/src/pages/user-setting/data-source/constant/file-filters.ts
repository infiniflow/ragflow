import { FormFieldType } from '@/components/dynamic-form';
import type { TFunction } from 'i18next';

// Sources opt in only after their traversal implements these exclusions.
export const getFileFilterFields = (t: TFunction) => [
  {
    label: t('setting.excludedExtensions'),
    name: 'config.excluded_extensions',
    type: FormFieldType.Tag,
    required: false,
    tooltip: t('setting.excludedExtensionsTip'),
  },
  {
    label: t('setting.excludedFolderNames'),
    name: 'config.excluded_folder_names',
    type: FormFieldType.Tag,
    required: false,
    tooltip: t('setting.excludedFolderNamesTip'),
  },
  {
    label: t('setting.excludedFolderPaths'),
    name: 'config.excluded_folder_paths',
    type: FormFieldType.Tag,
    required: false,
    tooltip: t('setting.excludedFolderPathsTip'),
  },
];
