import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const seafileConstant = (t: TFunction) => [
  {
    label: 'SeaFile Server URL',
    name: 'config.seafile_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://seafile.example.com',
    tooltip: t('setting.seafileUrlTip'),
  },
  {
    label: 'Sync Scope',
    name: 'config.sync_scope',
    type: FormFieldType.Segmented,
    options: [
      { label: 'Entire Account', value: 'account' },
      { label: 'Single Library', value: 'library' },
      { label: 'Specific Directory', value: 'directory' },
    ],
    tooltip: t('setting.seafileSyncScopeTip'),
  },

  {
    name: FilterFormField + '.account-tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope ?? 'account';
      return scope === 'account';
    },
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
        {t('setting.seafileAccountScopeTip')}
      </div>
    ),
  },
  {
    label: 'Account API Token',
    name: 'config.credentials.seafile_token',
    type: FormFieldType.Password,
    required: false,
    defaultValue: '',
    tooltip: t('setting.seafileTokenTip'),
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope ?? 'account';
      return scope === 'account';
    },
    customValidate: (val: string, formValues: any) => {
      const scope = formValues?.config?.sync_scope ?? 'account';
      if ((!val || val.trim() === '') && scope === 'account') {
        return t('setting.seafileValidationAccountTokenRequired');
      }
      return true;
    },
  },
  {
    label: 'Include Shared Libraries',
    name: 'config.include_shared',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: true,
    tooltip: t('setting.seafileIncludeSharedTip'),
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope ?? 'account';
      return scope === 'account';
    },
  },

  {
    // Contextual info panel explaining the two-token choice
    name: FilterFormField + '.token-tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      return scope === 'library' || scope === 'directory';
    },
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2 space-y-1">
        <p className="font-medium text-text-primary">
          {t('setting.seafileTokenPanelHeading')}
        </p>
        <ul className="list-disc list-inside space-y-0.5">
          <li>
            <span className="font-medium">Account API Token</span>
            {' ' + t('setting.seafileTokenPanelAccountBullet')}
          </li>
          <li>
            <span className="font-medium">Library Token</span>
            {' ' + t('setting.seafileTokenPanelLibraryBullet')}
          </li>
        </ul>
      </div>
    ),
  },
  {
    label: 'Account API Token',
    name: 'config.credentials.seafile_token',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.seafileTokenTip'),
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      return scope === 'library' || scope === 'directory';
    },
  },
  {
    label: 'Library Token',
    name: 'config.credentials.repo_token',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.seafileRepoTokenTip'),
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      return scope === 'library' || scope === 'directory';
    },
    customValidate: (val: string, formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      const accountToken = formValues?.config?.credentials?.seafile_token;
      if (
        !val &&
        !accountToken &&
        (scope === 'library' || scope === 'directory')
      ) {
        return t('setting.seafileValidationTokenRequired');
      }
      return true;
    },
  },
  {
    label: 'Library ID',
    name: 'config.repo_id',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'e.g. 7a9e1b3c-4d5f-6a7b-8c9d-0e1f2a3b4c5d',
    tooltip: t('setting.seafileRepoIdTip'),
    shouldRender: (formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      return scope === 'library' || scope === 'directory';
    },
    customValidate: (val: string, formValues: any) => {
      const scope = formValues?.config?.sync_scope;
      if (!val && (scope === 'library' || scope === 'directory')) {
        return t('setting.seafileValidationLibraryIdRequired');
      }
      return true;
    },
  },

  {
    label: 'Directory Path',
    name: 'config.sync_path',
    type: FormFieldType.Text,
    required: false,
    placeholder: '/Documents/Reports',
    tooltip: t('setting.seafileSyncPathTip'),
    shouldRender: (formValues: any) => {
      return formValues?.config?.sync_scope === 'directory';
    },
    customValidate: (val: string, formValues: any) => {
      if (!val && formValues?.config?.sync_scope === 'directory') {
        return t('setting.seafileValidationDirectoryPathRequired');
      }
      return true;
    },
  },

  {
    label: 'Batch Size',
    name: 'config.batch_size',
    type: FormFieldType.Number,
    required: false,
    placeholder: '100',
    tooltip: t('setting.seafileBatchSizeTip'),
  },

  {
    label: 'Account API Token',
    name: 'config.credentials.seafile_token',
    type: FormFieldType.Password,
    required: false,
    hidden: true,
  },
  {
    label: 'Library Token',
    name: 'config.credentials.repo_token',
    type: FormFieldType.Password,
    required: false,
    hidden: true,
  },
  {
    label: 'Library ID',
    name: 'config.repo_id',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: 'Directory Path',
    name: 'config.sync_path',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: 'Include Shared Libraries',
    name: 'config.include_shared',
    type: FormFieldType.Checkbox,
    required: false,
    hidden: true,
  },
];
