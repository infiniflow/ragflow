import { FormFieldType } from '@/components/dynamic-form';

export const alfrescoConstant = () => [
  {
    label: 'Base URL',
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://alfresco.example.com',
  },
  {
    label: 'Authentication',
    name: 'config.auth_mode',
    type: FormFieldType.Segmented,
    options: [
      { label: 'Basic Auth', value: 'basic' },
      { label: 'OAuth2', value: 'oauth2' },
    ],
    defaultValue: 'basic',
  },
  {
    label: 'Username',
    name: 'config.credentials.username',
    type: FormFieldType.Text,
    shouldRender: (formValues: any) =>
      (formValues?.config?.auth_mode ?? 'basic') === 'basic',
    customValidate: (val: string, formValues: any) => {
      const mode = formValues?.config?.auth_mode ?? 'basic';
      if (mode === 'basic' && !(val != null && String(val).trim())) {
        return 'Username is required';
      }
      return true;
    },
  },
  {
    label: 'Password',
    name: 'config.credentials.password',
    type: FormFieldType.Password,
    shouldRender: (formValues: any) =>
      (formValues?.config?.auth_mode ?? 'basic') === 'basic',
    customValidate: (val: string, formValues: any) => {
      const mode = formValues?.config?.auth_mode ?? 'basic';
      if (mode === 'basic' && !val) {
        return 'Password is required';
      }
      return true;
    },
  },
  {
    label: 'Access Token',
    name: 'config.credentials.access_token',
    type: FormFieldType.Password,
    shouldRender: (formValues: any) =>
      formValues?.config?.auth_mode === 'oauth2',
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.auth_mode === 'oauth2' && !val) {
        return 'Access Token is required';
      }
      return true;
    },
  },
  {
    label: 'Site IDs',
    name: 'config.site_ids',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'marketing,engineering',
    tooltip:
      'Comma-separated Alfresco site IDs to crawl (each site’s document library). Leave empty to crawl by folder or the whole repository.',
  },
  {
    label: 'Root Node IDs',
    name: 'config.root_node_ids',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'workspace://SpacesStore/...,-root-',
    tooltip:
      'Comma-separated folder node IDs to crawl. Leave empty (with no sites) to crawl the whole repository.',
  },
  {
    label: 'Include Version History',
    name: 'config.include_version_history',
    type: FormFieldType.Checkbox,
    required: false,
  },
];
