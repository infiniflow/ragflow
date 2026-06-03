import { FormFieldType } from '@/components/dynamic-form';

export const lookerConstant = () => [
  {
    label: 'Base URL',
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://yourcompany.looker.com',
  },
  {
    label: 'Client ID',
    name: 'config.credentials.client_id',
    type: FormFieldType.Text,
    required: true,
    tooltip: 'Looker API3 client_id.',
  },
  {
    label: 'Client Secret',
    name: 'config.credentials.client_secret',
    type: FormFieldType.Password,
    required: true,
    tooltip: 'Looker API3 client_secret.',
  },
  {
    label: 'Include Dashboards',
    name: 'config.include_dashboards',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: true,
  },
  {
    label: 'Include Looks',
    name: 'config.include_looks',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: true,
  },
  {
    label: 'Include Rendered CSV Exports',
    name: 'config.include_exports',
    type: FormFieldType.Checkbox,
    required: false,
    tooltip:
      'Also ingest each Look rendered to CSV (run_look). Off by default — exports are slower and larger.',
  },
];
