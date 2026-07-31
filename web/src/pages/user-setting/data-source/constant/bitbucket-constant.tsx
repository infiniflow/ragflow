import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const bitbucketConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldBitbucketAccountEmail'),
    name: 'config.credentials.bitbucket_account_email',
    type: FormFieldType.Email,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldBitbucketApiToken'),
    name: 'config.credentials.bitbucket_api_token',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldWorkspace'),
    name: 'config.workspace',
    type: FormFieldType.Text,
    required: true,
    tooltip: t('setting.bitbucketTopWorkspaceTip'),
  },
  {
    label: t('setting.dataSourceFieldIndexMode'),
    name: 'config.index_mode',
    type: FormFieldType.Segmented,
    options: [
      {
        label: t('setting.dataSourceOptionRepositories'),
        value: 'repositories',
      },
      { label: t('setting.dataSourceOptionProjects'), value: 'projects' },
      { label: t('setting.dataSourceOptionWorkspace'), value: 'workspace' },
    ],
  },
  {
    label: t('setting.dataSourceFieldRepositorySlugs'),
    name: 'config.repository_slugs',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val && index_mode === 'repositories') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldRepositorySlugs'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'repositories';
    },
    tooltip: t('setting.bitbucketRepositorySlugsTip'),
  },
  {
    label: t('setting.dataSourceFieldProjects'),
    name: 'config.projects',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val && index_mode === 'projects') {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldProjects'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      console.log('formValues.config', formValues?.config);
      return index_mode === 'projects';
    },
    tooltip: t('setting.bitbucketProjectsTip'),
  },
  {
    name: FilterFormField + '.tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      return index_mode === 'workspace';
    },
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
        {t('setting.bitbucketWorkspaceTip')}
      </div>
    ),
  },
];
