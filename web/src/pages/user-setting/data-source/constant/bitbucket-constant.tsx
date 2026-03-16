import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const bitbucketConstant = (t: TFunction) => [
  {
    label: 'Bitbucket Account Email',
    name: 'config.credentials.bitbucket_account_email',
    type: FormFieldType.Email,
    required: true,
  },
  {
    label: 'Bitbucket API Token',
    name: 'config.credentials.bitbucket_api_token',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: 'Workspace',
    name: 'config.workspace',
    type: FormFieldType.Text,
    required: true,
    tooltip: t('setting.bitbucketTopWorkspaceTip'),
  },
  {
    label: 'Index Mode',
    name: 'config.index_mode',
    type: FormFieldType.Segmented,
    options: [
      { label: 'Repositories', value: 'repositories' },
      { label: 'Project(s)', value: 'projects' },
      { label: 'Workspace', value: 'workspace' },
    ],
  },
  {
    label: 'Repository Slugs',
    name: 'config.repository_slugs',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val && index_mode === 'repositories') {
        return 'Repository Slugs is required';
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
    label: 'Projects',
    name: 'config.projects',
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const index_mode = formValues?.config?.index_mode;
      if (!val && index_mode === 'projects') {
        return 'Projects is required';
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
