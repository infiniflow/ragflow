import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const confluenceConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldConfluenceUsername'),
    name: 'config.credentials.confluence_username',
    type: FormFieldType.Text,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldConfluenceAccessToken'),
    name: 'config.credentials.confluence_access_token',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldWikiBaseUrl'),
    name: 'config.wiki_base',
    type: FormFieldType.Text,
    required: false,
    tooltip: t('setting.confluenceWikiBaseUrlTip'),
  },
  {
    label: t('setting.dataSourceFieldIsCloud'),
    name: 'config.is_cloud',
    type: FormFieldType.Checkbox,
    required: false,
    tooltip: t('setting.confluenceIsCloudTip'),
  },
  {
    label: t('setting.dataSourceFieldIndexMode'),
    name: 'config.index_mode',
    type: FormFieldType.Segmented,
    options: [
      { label: t('setting.dataSourceOptionEverything'), value: 'everything' },
      { label: t('setting.dataSourceOptionSpace'), value: 'space' },
      { label: t('setting.dataSourceOptionPage'), value: 'page' },
    ],
  },
  {
    name: 'config.page_id',
    label: t('setting.dataSourceFieldPageId'),
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      console.log('index_mode', index_mode, val);
      if (!val && index_mode === 'page') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldPageId'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'page';
    },
  },
  {
    name: 'config.space',
    label: t('setting.dataSourceFieldSpaceKey'),
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val && index_mode === 'space') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldSpaceKey'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'space';
    },
  },
  {
    name: 'config.index_recursively',
    label: t('setting.dataSourceFieldIndexRecursively'),
    type: FormFieldType.Checkbox,
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'page';
    },
  },
  {
    name: FilterFormField + '.tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'everything';
    },
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
        {t('setting.dataSourceConfluenceEverythingTip')}
      </div>
    ),
  },
  {
    label: t('setting.dataSourceFieldSpaceKey'),
    name: 'config.space',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: t('setting.dataSourceFieldPageId'),
    name: 'config.page_id',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: t('setting.dataSourceFieldIndexRecursively'),
    name: 'config.index_recursively',
    type: FormFieldType.Checkbox,
    required: false,
    hidden: true,
  },
];
