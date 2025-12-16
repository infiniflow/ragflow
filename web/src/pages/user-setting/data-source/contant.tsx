import { FormFieldType } from '@/components/dynamic-form';
import SvgIcon from '@/components/svg-icon';
import { t, TFunction } from 'i18next';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import BoxTokenField from './component/box-token-field';
import { ConfluenceIndexingModeField } from './component/confluence-token-field';
import GmailTokenField from './component/gmail-token-field';
import GoogleDriveTokenField from './component/google-drive-token-field';
import { IDataSourceInfoMap } from './interface';
export enum DataSourceKey {
  CONFLUENCE = 'confluence',
  S3 = 's3',
  NOTION = 'notion',
  DISCORD = 'discord',
  GOOGLE_DRIVE = 'google_drive',
  MOODLE = 'moodle',
  GMAIL = 'gmail',
  JIRA = 'jira',
  WEBDAV = 'webdav',
  BOX = 'box',
  DROPBOX = 'dropbox',
  //   SHAREPOINT = 'sharepoint',
  //   SLACK = 'slack',
  //   TEAMS = 'teams',
}

export const generateDataSourceInfo = (t: TFunction) => {
  return {
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
    [DataSourceKey.GOOGLE_DRIVE]: {
      name: 'Google Drive',
      description: t(`setting.${DataSourceKey.GOOGLE_DRIVE}Description`),
      icon: <SvgIcon name={'data-source/google-drive'} width={38} />,
    },
    [DataSourceKey.GMAIL]: {
      name: 'Gmail',
      description: t(`setting.${DataSourceKey.GMAIL}Description`),
      icon: <SvgIcon name={'data-source/gmail'} width={38} />,
    },
    [DataSourceKey.MOODLE]: {
      name: 'Moodle',
      description: t(`setting.${DataSourceKey.MOODLE}Description`),
      icon: <SvgIcon name={'data-source/moodle'} width={38} />,
    },
    [DataSourceKey.JIRA]: {
      name: 'Jira',
      description: t(`setting.${DataSourceKey.JIRA}Description`),
      icon: <SvgIcon name={'data-source/jira'} width={38} />,
    },
    [DataSourceKey.WEBDAV]: {
      name: 'WebDAV',
      description: t(`setting.${DataSourceKey.WEBDAV}Description`),
      icon: <SvgIcon name={'data-source/webdav'} width={38} />,
    },
    [DataSourceKey.DROPBOX]: {
      name: 'Dropbox',
      description: t(`setting.${DataSourceKey.DROPBOX}Description`),
      icon: <SvgIcon name={'data-source/dropbox'} width={38} />,
    },
    [DataSourceKey.BOX]: {
      name: 'Box',
      description: t(`setting.${DataSourceKey.BOX}Description`),
      icon: <SvgIcon name={'data-source/box'} width={38} />,
    },
  };
};

export const useDataSourceInfo = () => {
  const { t } = useTranslation();
  const [dataSourceInfo, setDataSourceInfo] = useState<IDataSourceInfoMap>(
    generateDataSourceInfo(t) as IDataSourceInfoMap,
  );
  useEffect(() => {
    setDataSourceInfo(generateDataSourceInfo(t));
  }, [t]);
  return { dataSourceInfo };
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
        { label: 'S3 Compatible', value: 's3_compatible' },
      ],
      required: true,
    },
    {
      label: 'Addressing Style',
      name: 'config.credentials.addressing_style',
      type: FormFieldType.Select,
      options: [
        { label: 'Virtual Hosted Style', value: 'virtual' },
        { label: 'Path Style', value: 'path' },
      ],
      required: false,
      placeholder: 'Virtual Hosted Style',
      tooltip: t('setting.S3CompatibleAddressingStyleTip'),
      shouldRender: (formValues: any) => {
        return formValues?.config?.bucket_type === 's3_compatible';
      },
    },
    {
      label: 'Endpoint URL',
      name: 'config.credentials.endpoint_url',
      type: FormFieldType.Text,
      required: false,
      placeholder: 'https://fsn1.your-objectstorage.com',
      tooltip: t('setting.S3CompatibleEndpointUrlTip'),
      shouldRender: (formValues: any) => {
        return formValues?.config?.bucket_type === 's3_compatible';
      },
    },
    {
      label: 'Prefix',
      name: 'config.prefix',
      type: FormFieldType.Text,
      required: false,
      tooltip: t('setting.s3PrefixTip'),
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
      tooltip: t('setting.confluenceWikiBaseUrlTip'),
    },
    {
      label: 'Is Cloud',
      name: 'config.is_cloud',
      type: FormFieldType.Checkbox,
      required: false,
      tooltip: t('setting.confluenceIsCloudTip'),
    },
    {
      label: 'Index Method',
      name: 'config.index_mode',
      type: FormFieldType.Text,
      required: false,
      horizontal: true,
      labelClassName: 'self-start pt-4',
      render: (fieldProps: any) => (
        <ConfluenceIndexingModeField {...fieldProps} />
      ),
    },
    {
      label: 'Space Key',
      name: 'config.space',
      type: FormFieldType.Text,
      required: false,
      hidden: true,
    },
    {
      label: 'Page ID',
      name: 'config.page_id',
      type: FormFieldType.Text,
      required: false,
      hidden: true,
    },
    {
      label: 'Index Recursively',
      name: 'config.index_recursively',
      type: FormFieldType.Checkbox,
      required: false,
      hidden: true,
    },
  ],
  [DataSourceKey.GOOGLE_DRIVE]: [
    {
      label: 'Primary Admin Email',
      name: 'config.credentials.google_primary_admin',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'admin@example.com',
      tooltip: t('setting.google_drivePrimaryAdminTip'),
    },
    {
      label: 'OAuth Token JSON',
      name: 'config.credentials.google_tokens',
      type: FormFieldType.Textarea,
      required: true,
      render: (fieldProps: any) => (
        <GoogleDriveTokenField
          value={fieldProps.value}
          onChange={fieldProps.onChange}
          placeholder='{ "token": "...", "refresh_token": "...", ... }'
        />
      ),
      tooltip: t('setting.google_driveTokenTip'),
    },
    {
      label: 'My Drive Emails',
      name: 'config.my_drive_emails',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'user1@example.com,user2@example.com',
      tooltip: t('setting.google_driveMyDriveEmailsTip'),
    },
    {
      label: 'Shared Folder URLs',
      name: 'config.shared_folder_urls',
      type: FormFieldType.Textarea,
      required: true,
      placeholder:
        'https://drive.google.com/drive/folders/XXXXX,https://drive.google.com/drive/folders/YYYYY',
      tooltip: t('setting.google_driveSharedFoldersTip'),
    },
    // The fields below are intentionally disabled for now. Uncomment them when we
    // reintroduce shared drive controls or advanced impersonation options.
    // {
    //   label: 'Shared Drive URLs',
    //   name: 'config.shared_drive_urls',
    //   type: FormFieldType.Text,
    //   required: false,
    //   placeholder:
    //     'Optional: comma-separated shared drive links if you want to include them.',
    // },
    // {
    //   label: 'Specific User Emails',
    //   name: 'config.specific_user_emails',
    //   type: FormFieldType.Text,
    //   required: false,
    //   placeholder:
    //     'Optional: comma-separated list of users to impersonate (overrides defaults).',
    // },
    // {
    //      label: 'Include My Drive',
    //      name: 'config.include_my_drives',
    //      type: FormFieldType.Checkbox,
    //      required: false,
    //      defaultValue: true,
    // },
    // {
    //   label: 'Include Shared Drives',
    //   name: 'config.include_shared_drives',
    //   type: FormFieldType.Checkbox,
    //   required: false,
    //   defaultValue: false,
    // },
    // {
    //   label: 'Include “Shared with me”',
    //   name: 'config.include_files_shared_with_me',
    //   type: FormFieldType.Checkbox,
    //   required: false,
    //   defaultValue: false,
    // },
    // {
    //   label: 'Allow Images',
    //   name: 'config.allow_images',
    //   type: FormFieldType.Checkbox,
    //   required: false,
    //   defaultValue: false,
    // },
    {
      label: '',
      name: 'config.credentials.authentication_method',
      type: FormFieldType.Text,
      required: false,
      hidden: true,
      defaultValue: 'uploaded',
    },
  ],
  [DataSourceKey.GMAIL]: [
    {
      label: 'Primary Admin Email',
      name: 'config.credentials.google_primary_admin',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'admin@example.com',
      tooltip: t('setting.gmailPrimaryAdminTip'),
    },
    {
      label: 'OAuth Token JSON',
      name: 'config.credentials.google_tokens',
      type: FormFieldType.Textarea,
      required: true,
      render: (fieldProps: any) => (
        <GmailTokenField
          value={fieldProps.value}
          onChange={fieldProps.onChange}
          placeholder='{ "token": "...", "refresh_token": "...", ... }'
        />
      ),
      tooltip: t('setting.gmailTokenTip'),
    },
    {
      label: '',
      name: 'config.credentials.authentication_method',
      type: FormFieldType.Text,
      required: false,
      hidden: true,
      defaultValue: 'uploaded',
    },
  ],
  [DataSourceKey.MOODLE]: [
    {
      label: 'Moodle URL',
      name: 'config.moodle_url',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'https://moodle.example.com',
    },
    {
      label: 'API Token',
      name: 'config.credentials.moodle_token',
      type: FormFieldType.Password,
      required: true,
    },
  ],
  [DataSourceKey.JIRA]: [
    {
      label: 'Jira Base URL',
      name: 'config.base_url',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'https://your-domain.atlassian.net',
      tooltip: t('setting.jiraBaseUrlTip'),
    },
    {
      label: 'Project Key',
      name: 'config.project_key',
      type: FormFieldType.Text,
      required: false,
      placeholder: 'RAGFlow',
      tooltip: t('setting.jiraProjectKeyTip'),
    },
    {
      label: 'Custom JQL',
      name: 'config.jql_query',
      type: FormFieldType.Textarea,
      required: false,
      placeholder: 'project = RAG AND updated >= -7d',
      tooltip: t('setting.jiraJqlTip'),
    },
    {
      label: 'Batch Size',
      name: 'config.batch_size',
      type: FormFieldType.Number,
      required: false,
      tooltip: t('setting.jiraBatchSizeTip'),
    },
    {
      label: 'Include Comments',
      name: 'config.include_comments',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: true,
      tooltip: t('setting.jiraCommentsTip'),
    },
    {
      label: 'Include Attachments',
      name: 'config.include_attachments',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: false,
      tooltip: t('setting.jiraAttachmentsTip'),
    },
    {
      label: 'Attachment Size Limit (bytes)',
      name: 'config.attachment_size_limit',
      type: FormFieldType.Number,
      required: false,
      defaultValue: 10 * 1024 * 1024,
      tooltip: t('setting.jiraAttachmentSizeTip'),
    },
    {
      label: 'Labels to Skip',
      name: 'config.labels_to_skip',
      type: FormFieldType.Tag,
      required: false,
      tooltip: t('setting.jiraLabelsTip'),
    },
    {
      label: 'Comment Email Blacklist',
      name: 'config.comment_email_blacklist',
      type: FormFieldType.Tag,
      required: false,
      tooltip: t('setting.jiraBlacklistTip'),
    },
    {
      label: 'Use Scoped Token (Clould only)',
      name: 'config.scoped_token',
      type: FormFieldType.Checkbox,
      required: false,
      tooltip: t('setting.jiraScopedTokenTip'),
    },
    {
      label: 'Jira User Email (Cloud) or User Name (Server)',
      name: 'config.credentials.jira_user_email',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'you@example.com',
      tooltip: t('setting.jiraEmailTip'),
    },
    {
      label: 'Jira API Token (Cloud only)',
      name: 'config.credentials.jira_api_token',
      type: FormFieldType.Password,
      required: false,
      tooltip: t('setting.jiraTokenTip'),
    },
    {
      label: 'Jira Password (Server only)',
      name: 'config.credentials.jira_password',
      type: FormFieldType.Password,
      required: false,
      tooltip: t('setting.jiraPasswordTip'),
    },
  ],
  [DataSourceKey.WEBDAV]: [
    {
      label: 'WebDAV Server URL',
      name: 'config.base_url',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'https://webdav.example.com',
    },
    {
      label: 'Username',
      name: 'config.credentials.username',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Password',
      name: 'config.credentials.password',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Remote Path',
      name: 'config.remote_path',
      type: FormFieldType.Text,
      required: false,
      placeholder: '/',
      tooltip: t('setting.webdavRemotePathTip'),
    },
  ],
  [DataSourceKey.DROPBOX]: [
    {
      label: 'Access Token',
      name: 'config.credentials.dropbox_access_token',
      type: FormFieldType.Password,
      required: true,
      tooltip: t('setting.dropboxAccessTokenTip'),
    },
    {
      label: 'Batch Size',
      name: 'config.batch_size',
      type: FormFieldType.Number,
      required: false,
      placeholder: 'Defaults to 2',
    },
  ],
  [DataSourceKey.BOX]: [
    {
      label: 'Box OAuth JSON',
      name: 'config.credentials.box_tokens',
      type: FormFieldType.Textarea,
      required: true,
      render: (fieldProps: any) => (
        <BoxTokenField
          value={fieldProps.value}
          onChange={fieldProps.onChange}
          placeholder='{ "client_id": "...", "client_secret": "...", "redirect_uri": "..." }'
        />
      ),
    },
    {
      label: 'Folder ID',
      name: 'config.folder_id',
      type: FormFieldType.Text,
      required: false,
      placeholder: 'Defaults root',
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
        endpoint_url: '',
        addressing_style: 'virtual',
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
      space: '',
      credentials: {
        confluence_username: '',
        confluence_access_token: '',
      },
      index_mode: 'everything',
    },
  },
  [DataSourceKey.GOOGLE_DRIVE]: {
    name: '',
    source: DataSourceKey.GOOGLE_DRIVE,
    config: {
      include_shared_drives: false,
      include_my_drives: true,
      include_files_shared_with_me: false,
      allow_images: false,
      shared_drive_urls: '',
      shared_folder_urls: '',
      my_drive_emails: '',
      specific_user_emails: '',
      credentials: {
        google_primary_admin: '',
        google_tokens: '',
        authentication_method: 'uploaded',
      },
    },
  },
  [DataSourceKey.GMAIL]: {
    name: '',
    source: DataSourceKey.GMAIL,
    config: {
      credentials: {
        google_primary_admin: '',
        google_tokens: '',
        authentication_method: 'uploaded',
      },
    },
  },
  [DataSourceKey.MOODLE]: {
    name: '',
    source: DataSourceKey.MOODLE,
    config: {
      moodle_url: '',
      credentials: {
        moodle_token: '',
      },
    },
  },
  [DataSourceKey.JIRA]: {
    name: '',
    source: DataSourceKey.JIRA,
    config: {
      base_url: '',
      project_key: '',
      jql_query: '',
      batch_size: 2,
      include_comments: true,
      include_attachments: false,
      attachment_size_limit: 10 * 1024 * 1024,
      labels_to_skip: [],
      comment_email_blacklist: [],
      scoped_token: false,
      credentials: {
        jira_user_email: '',
        jira_api_token: '',
        jira_password: '',
      },
    },
  },
  [DataSourceKey.WEBDAV]: {
    name: '',
    source: DataSourceKey.WEBDAV,
    config: {
      base_url: '',
      remote_path: '/',
      credentials: {
        username: '',
        password: '',
      },
    },
  },
  [DataSourceKey.DROPBOX]: {
    name: '',
    source: DataSourceKey.DROPBOX,
    config: {
      batch_size: 2,
      credentials: {
        dropbox_access_token: '',
      },
    },
  },
  [DataSourceKey.BOX]: {
    name: '',
    source: DataSourceKey.BOX,
    config: {
      name: '',
      folder_id: '0',
      credentials: {
        box_tokens: '',
      },
    },
  },
};
