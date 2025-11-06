import SvgIcon from '@/components/svg-icon';
import { t } from 'i18next';
import { FormFieldType } from './component/dynamic-form';

export enum DataSourceKey {
  S3 = 's3',
  NOTION = 'notion',
  DISCORD = 'discord',
  //   CONFLUENNCE = 'confluence',
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
    icon: <SvgIcon name={'data-source/s3'} width={28} />,
  },
  [DataSourceKey.NOTION]: {
    name: 'Notion',
    description: t(`setting.${DataSourceKey.NOTION}Description`),
    icon: <SvgIcon name={'data-source/notion'} width={28} />,
  },
  [DataSourceKey.DISCORD]: {
    name: 'Discord',
    description: t(`setting.${DataSourceKey.DISCORD}Description`),
    icon: <SvgIcon name={'data-source/discord'} width={28} />,
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
      type: FormFieldType.Text,
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
      type: FormFieldType.Text,
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
      type: FormFieldType.Text,
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
};
