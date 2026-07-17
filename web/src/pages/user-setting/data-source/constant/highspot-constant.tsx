import { FormFieldType } from '@/components/dynamic-form';

export const highspotConstant = () => [
  {
    label: 'API Key',
    name: 'config.credentials.api_key',
    type: FormFieldType.Text,
    required: true,
  },
  {
    label: 'API Secret',
    name: 'config.credentials.api_secret',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: 'Base URL',
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'https://api.highspot.com',
    tooltip:
      'Highspot REST API base URL. Leave empty to use https://api.highspot.com.',
  },
  {
    label: 'Spot IDs',
    name: 'config.spot_ids',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'spotId1, spotId2',
    tooltip:
      'Comma-separated Spot IDs to sync. Leave empty to sync every Spot the API key can access.',
  },
  {
    label: 'Include Downloadable Files',
    name: 'config.include_files',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: true,
  },
];
