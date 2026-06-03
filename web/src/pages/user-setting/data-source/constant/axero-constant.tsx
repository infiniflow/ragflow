import { FormFieldType } from '@/components/dynamic-form';

export const axeroConstant = () => [
  {
    label: 'Base URL',
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://intranet.example.com',
  },
  {
    label: 'REST API Key',
    name: 'config.credentials.api_key',
    type: FormFieldType.Password,
    required: true,
  },
  {
    label: 'Space IDs',
    name: 'config.space_ids',
    type: FormFieldType.Text,
    required: false,
    placeholder: '12,34',
    tooltip:
      'Comma-separated Axero space IDs to sync. Leave empty to sync every space the API key can access.',
  },
  {
    label: 'Content Types',
    name: 'config.content_types',
    type: FormFieldType.MultiSelect,
    required: true,
    options: [
      { label: 'Articles', value: 'article' },
      { label: 'Wiki Pages', value: 'wiki' },
      { label: 'Blog Posts', value: 'blog' },
      { label: 'Forum Threads', value: 'forum' },
    ],
    defaultValue: ['article', 'wiki', 'blog', 'forum'],
  },
  {
    label: 'Include Attached Files',
    name: 'config.include_attachments',
    type: FormFieldType.Checkbox,
    required: false,
  },
];
