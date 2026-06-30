import { FormFieldType } from '@/components/dynamic-form';

export const lumappsConstant = () => [
  {
    label: 'Base URL',
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://sites.lumapps.com',
    tooltip: 'Your LumApps organization / API base URL.',
  },
  {
    label: 'Access Token',
    name: 'config.credentials.access_token',
    type: FormFieldType.Password,
    required: false,
    tooltip:
      'OAuth2 bearer token. Provide this, or supply Client ID + Client Secret + Token URL below for a client-credentials grant.',
  },
  {
    label: 'Client ID',
    name: 'config.credentials.client_id',
    type: FormFieldType.Text,
    required: false,
  },
  {
    label: 'Client Secret',
    name: 'config.credentials.client_secret',
    type: FormFieldType.Password,
    required: false,
  },
  {
    label: 'Token URL',
    name: 'config.token_url',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'https://login.lumapps.com/oauth/token',
    tooltip:
      'OAuth2 token endpoint, used with Client ID + Client Secret when no access token is supplied.',
  },
  {
    label: 'Community IDs',
    name: 'config.community_ids',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'communityId1, communityId2',
    tooltip:
      'Comma-separated community/instance IDs to sync. Leave empty to sync all accessible content.',
  },
  {
    label: 'Content Types',
    name: 'config.content_types',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'article, post, page',
    tooltip:
      'Comma-separated LumApps content types to ingest. Leave empty for all types.',
  },
  {
    label: 'Language',
    name: 'config.lang',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'en',
    tooltip: 'Preferred language for localized fields (title, body).',
  },
  {
    label: 'Include Attachments',
    name: 'config.include_attachments',
    type: FormFieldType.Checkbox,
    required: false,
  },
];
