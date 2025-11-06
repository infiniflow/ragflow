import { FormFieldType } from '@/components/dynamic-form';
import SvgIcon from '@/components/svg-icon';
import { t } from 'i18next';

export enum DataSourceKey {
  CONFLUENCE = 'confluence',
  S3 = 's3',
  NOTION = 'notion',
  DISCORD = 'discord',
  //   GMAIL = 'gmail',
  //   GOOGLE_DRIVER = 'google_driver',
  //   JIRA = 'jira',
  //   SHAREPOINT = 'sharepoint',
  //   SLACK = 'slack',
  //   TEAMS = 'teams',
}

export const DataSourceInfo = {
  [DataSourceKey.S3]: {
    name: 'S3',
    description: t(`setting.${DataSourceKey.S3}Description`),
    icon: <SvgIcon name={'data-source/s3'} width={38} />,
  },
  [DataSourceKey.NOTION]: {
    name: 'Notion',
    description: t(`setting.${DataSourceKey.NOTION}Description`),
    icon: <SvgIcon name={'data-source/notion'} width={38} />,
  },
  [DataSourceKey.DISCORD]: {
    name: 'Discord',
    description: t(`setting.${DataSourceKey.DISCORD}Description`),
    icon: <SvgIcon name={'data-source/discord'} width={38} />,
  },
  [DataSourceKey.CONFLUENCE]: {
    name: 'Confluence',
    description: t(`setting.${DataSourceKey.CONFLUENCE}Description`),
    icon: <SvgIcon name={'data-source/confluence'} width={38} />,
  },
};

export const DataSourceFormBaseFields = [
  {
    id: 'Id',
    name: 'id',
    type: FormFieldType.Text,
    required: false,
    hidden: true,
  },
  {
    label: 'Name',
    name: 'name',
    type: FormFieldType.Text,
    required: true,
  },
  {
    label: 'Source',
    name: 'source',
    type: FormFieldType.Select,
    required: true,
    hidden: true,
    options: Object.keys(DataSourceKey).map((item) => ({
      label: item,
      value: DataSourceKey[item as keyof typeof DataSourceKey],
    })),
  },
];

export const DataSourceFormFields = {
  [DataSourceKey.S3]: [
    {
      label: 'AWS Access Key ID',
      name: 'config.credentials.aws_access_key_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'AWS Secret Access Key',
      name: 'config.credentials.aws_secret_access_key',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Bucket Name',
      name: 'config.bucket_name',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Bucket Type',
      name: 'config.bucket_type',
      type: FormFieldType.Select,
      options: [
        { label: 'S3', value: 's3' },
        { label: 'R2', value: 'r2' },
        { label: 'Google Cloud Storage', value: 'google_cloud_storage' },
        { label: 'OCI Storage', value: 'oci_storage' },
      ],
      required: true,
    },
    {
      label: 'Prefix',
      name: 'config.prefix',
      type: FormFieldType.Text,
      required: false,
    },
  ],
  [DataSourceKey.NOTION]: [
    {
      label: 'Notion Integration Token',
      name: 'config.credentials.notion_integration_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Root Page Id',
      name: 'config.root_page_id',
      type: FormFieldType.Text,
      required: false,
    },
  ],
  [DataSourceKey.DISCORD]: [
    {
      label: 'Discord Bot Token',
      name: 'config.credentials.discord_bot_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Server IDs',
      name: 'config.server_ids',
      type: FormFieldType.Tag,
      required: false,
    },
    {
      label: 'Channels',
      name: 'config.channels',
      type: FormFieldType.Tag,
      required: false,
    },
  ],

  [DataSourceKey.CONFLUENCE]: [
    {
      label: 'Confluence Username',
      name: 'config.credentials.confluence_username',
      type: FormFieldType.Text,
      required: true,
      tooltip: 'A descriptive name for the connector.',
    },
    {
      label: 'Confluence Access Token',
      name: 'config.credentials.confluence_access_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Wiki Base URL',
      name: 'config.wiki_base',
      type: FormFieldType.Text,
      required: false,
      tooltip:
        'The base URL of your Confluence instance (e.g., https://your-domain.atlassian.net/wiki)',
    },
    {
      label: 'Is Cloud',
      name: 'config.is_cloud',
      type: FormFieldType.Checkbox,
      required: false,
      tooltip:
        'Check if this is a Confluence Cloud instance, uncheck for Confluence Server/Data Center',
    },
  ],
};

export const DataSourceFormDefaultValues = {
  [DataSourceKey.S3]: {
    name: '',
    source: DataSourceKey.S3,
    config: {
      bucket_name: '',
      bucket_type: 's3',
      prefix: '',
      credentials: {
        aws_access_key_id: '',
        aws_secret_access_key: '',
      },
    },
  },
  [DataSourceKey.NOTION]: {
    name: '',
    source: DataSourceKey.NOTION,
    config: {
      root_page_id: '',
      credentials: {
        notion_integration_token: '',
      },
    },
  },
  [DataSourceKey.DISCORD]: {
    name: '',
    source: DataSourceKey.DISCORD,
    config: {
      server_ids: [],
      channels: [],
      credentials: {
        discord_bot_token: '',
      },
    },
  },
  [DataSourceKey.CONFLUENCE]: {
    name: '',
    source: DataSourceKey.CONFLUENCE,
    config: {
      wiki_base: '',
      is_cloud: true,
      credentials: {
        confluence_username: '',
        confluence_access_token: '',
      },
    },
  },
};
