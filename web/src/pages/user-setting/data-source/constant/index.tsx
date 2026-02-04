import { FormFieldType } from '@/components/dynamic-form';
import { IconFontFill } from '@/components/icon-font';
import SvgIcon from '@/components/svg-icon';
import { t, TFunction } from 'i18next';
import { Mail } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import BoxTokenField from '../component/box-token-field';
import GmailTokenField from '../component/gmail-token-field';
import GoogleDriveTokenField from '../component/google-drive-token-field';
import { IDataSourceInfoMap } from '../interface';
import { bitbucketConstant } from './bitbucket-constant';
import { confluenceConstant } from './confluence-constant';
import { S3Constant } from './s3-constant';

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
  R2 = 'r2',
  OCI_STORAGE = 'oci_storage',
  GOOGLE_CLOUD_STORAGE = 'google_cloud_storage',
  AIRTABLE = 'airtable',
  GITLAB = 'gitlab',
  ASANA = 'asana',
  IMAP = 'imap',
  GITHUB = 'github',
  BITBUCKET = 'bitbucket',
  ZENDESK = 'zendesk',
  SEAFILE = 'seafile',
  MYSQL = 'mysql',
  POSTGRESQL = 'postgresql',
  //   SHAREPOINT = 'sharepoint',
  //   SLACK = 'slack',
  //   TEAMS = 'teams',
}

export const generateDataSourceInfo = (t: TFunction) => {
  return {
    [DataSourceKey.GOOGLE_CLOUD_STORAGE]: {
      name: 'Google Cloud Storage',
      description: t(
        `setting.${DataSourceKey.GOOGLE_CLOUD_STORAGE}Description`,
      ),
      icon: <SvgIcon name={'data-source/google-cloud-storage'} width={38} />,
    },
    [DataSourceKey.OCI_STORAGE]: {
      name: 'Oracle Storage',
      description: t(`setting.${DataSourceKey.OCI_STORAGE}Description`),
      icon: <SvgIcon name={'data-source/oracle-storage'} width={38} />,
    },
    [DataSourceKey.R2]: {
      name: 'R2',
      description: t(`setting.${DataSourceKey.R2}Description`),
      icon: <SvgIcon name={'data-source/r2'} width={38} />,
    },
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
    [DataSourceKey.AIRTABLE]: {
      name: 'Airtable',
      description: t(`setting.${DataSourceKey.AIRTABLE}Description`),
      icon: <SvgIcon name={'data-source/airtable'} width={38} />,
    },
    [DataSourceKey.GITLAB]: {
      name: 'GitLab',
      description: t(`setting.${DataSourceKey.GITLAB}Description`),
      icon: <SvgIcon name={'data-source/gitlab'} width={38} />,
    },
    [DataSourceKey.ASANA]: {
      name: 'Asana',
      description: t(`setting.${DataSourceKey.ASANA}Description`),
      icon: <SvgIcon name={'data-source/asana'} width={38} />,
    },
    [DataSourceKey.GITHUB]: {
      name: 'GitHub',
      description: t(`setting.${DataSourceKey.GITHUB}Description`),
      icon: (
        <IconFontFill
          // name="a-DiscordIconSVGVectorIcon"
          name="GitHub"
          className="text-text-primary size-6"
        ></IconFontFill>
      ),
    },
    [DataSourceKey.IMAP]: {
      name: 'IMAP',
      description: t(`setting.${DataSourceKey.IMAP}Description`),
      icon: <Mail className="text-text-primary" size={22} />,
    },
    [DataSourceKey.BITBUCKET]: {
      name: 'Bitbucket',
      description: t(`setting.${DataSourceKey.BITBUCKET}Description`),
      icon: <SvgIcon name={'data-source/bitbucket'} width={38} />,
    },
    [DataSourceKey.ZENDESK]: {
      name: 'Zendesk',
      description: t(`setting.${DataSourceKey.ZENDESK}Description`),
      icon: <SvgIcon name={'data-source/zendesk'} width={38} />,
    },
    [DataSourceKey.SEAFILE]: {
      name: 'SeaFile',
      description: t(`setting.${DataSourceKey.SEAFILE}Description`),
      icon: <SvgIcon name={'data-source/seafile'} width={38} />,
    },
    [DataSourceKey.MYSQL]: {
      name: 'MySQL',
      description: t(`setting.${DataSourceKey.MYSQL}Description`),
      icon: <SvgIcon name={'data-source/mysql'} width={38} />,
    },
    [DataSourceKey.POSTGRESQL]: {
      name: 'PostgreSQL',
      description: t(`setting.${DataSourceKey.POSTGRESQL}Description`),
      icon: <SvgIcon name={'data-source/postgresql'} width={38} />,
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
    tooltip: t('setting.connectorNameTip'),
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
  [DataSourceKey.GOOGLE_CLOUD_STORAGE]: [
    {
      label: 'GCS Access Key ID',
      name: 'config.credentials.access_key_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'GCS Secret Access Key',
      name: 'config.credentials.secret_access_key',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Bucket Name',
      name: 'config.bucket_name',
      type: FormFieldType.Text,
      required: true,
    },
  ],
  [DataSourceKey.OCI_STORAGE]: [
    {
      label: 'OCI Namespace',
      name: 'config.credentials.namespace',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'OCI Region',
      name: 'config.credentials.region',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'OCI Access Key ID',
      name: 'config.credentials.access_key_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'OCI Secret Access Key',
      name: 'config.credentials.secret_access_key',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Bucket Name',
      name: 'config.bucket_name',
      type: FormFieldType.Text,
      required: true,
    },
  ],
  [DataSourceKey.R2]: [
    {
      label: 'R2 Account ID',
      name: 'config.credentials.account_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'R2 Access Key ID',
      name: 'config.credentials.r2_access_key_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'R2 Secret Access Key',
      name: 'config.credentials.r2_secret_access_key',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Bucket Name',
      name: 'config.bucket_name',
      type: FormFieldType.Text,
      required: true,
    },
  ],
  [DataSourceKey.S3]: S3Constant(t),
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

  [DataSourceKey.CONFLUENCE]: confluenceConstant(t),
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
  [DataSourceKey.AIRTABLE]: [
    {
      label: 'Access Token',
      name: 'config.credentials.airtable_access_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Base ID',
      name: 'config.base_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Table Name OR ID',
      name: 'config.table_name_or_id',
      type: FormFieldType.Text,
      required: true,
    },
  ],
  [DataSourceKey.GITLAB]: [
    {
      label: 'Project Owner',
      name: 'config.project_owner',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Project Name',
      name: 'config.project_name',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'GitLab Personal Access Token',
      name: 'config.credentials.gitlab_access_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'GitLab URL',
      name: 'config.gitlab_url',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'https://gitlab.com',
    },
    {
      label: 'include Merge Requests',
      name: 'config.include_mrs',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: true,
    },
    {
      label: 'include Issues',
      name: 'config.include_issues',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: true,
    },
    {
      label: 'include Code Files',
      name: 'config.include_code_files',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: true,
    },
  ],
  [DataSourceKey.ASANA]: [
    {
      label: 'API Token',
      name: 'config.credentials.asana_api_token_secret',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Workspace ID',
      name: 'config.asana_workspace_id',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Project IDs',
      name: 'config.asana_project_ids',
      type: FormFieldType.Text,
      required: false,
    },
    {
      label: 'Team ID',
      name: 'config.asana_team_id',
      type: FormFieldType.Text,
      required: false,
    },
  ],
  [DataSourceKey.GITHUB]: [
    {
      label: 'Repository Owner',
      name: 'config.repository_owner',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Repository Name',
      name: 'config.repository_name',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'GitHub Access Token',
      name: 'config.credentials.github_access_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Inlcude Pull Requests',
      name: 'config.include_pull_requests',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: false,
    },
    {
      label: 'Inlcude Issues',
      name: 'config.include_issues',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: false,
    },
  ],
  [DataSourceKey.IMAP]: [
    {
      label: 'Username',
      name: 'config.credentials.imap_username',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Password',
      name: 'config.credentials.imap_password',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Host',
      name: 'config.imap_host',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Port',
      name: 'config.imap_port',
      type: FormFieldType.Number,
      required: true,
    },
    {
      label: 'Mailboxes',
      name: 'config.imap_mailbox',
      type: FormFieldType.Tag,
      required: false,
    },
    {
      label: 'Poll Range',
      name: 'config.poll_range',
      type: FormFieldType.Number,
      required: false,
    },
  ],
  [DataSourceKey.BITBUCKET]: bitbucketConstant(t),
  [DataSourceKey.ZENDESK]: [
    {
      label: 'Zendesk Domain',
      name: 'config.credentials.zendesk_subdomain',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Zendesk Email',
      name: 'config.credentials.zendesk_email',
      type: FormFieldType.Text,
      required: true,
    },
    {
      label: 'Zendesk Token',
      name: 'config.credentials.zendesk_token',
      type: FormFieldType.Password,
      required: true,
    },
    {
      label: 'Content',
      name: 'config.zendesk_content_type',
      type: FormFieldType.Segmented,
      required: true,
      options: [
        { label: 'Articles', value: 'articles' },
        { label: 'Tickets', value: 'tickets' },
      ],
    },
  ],
  [DataSourceKey.SEAFILE]: [
    {
      label: 'SeaFile Server URL',
      name: 'config.seafile_url',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'https://seafile.example.com',
      tooltip: t('setting.seafileUrlTip'),
    },
    {
      label: 'API Token',
      name: 'config.credentials.seafile_token',
      type: FormFieldType.Password,
      required: true,
      tooltip: t('setting.seafileTokenTip'),
    },
    {
      label: 'Include Shared Libraries',
      name: 'config.include_shared',
      type: FormFieldType.Checkbox,
      required: false,
      defaultValue: true,
      tooltip: t('setting.seafileIncludeSharedTip'),
    },
    {
      label: 'Batch Size',
      name: 'config.batch_size',
      type: FormFieldType.Number,
      required: false,
      placeholder: '100',
      tooltip: t('setting.seafileBatchSizeTip'),
    },
  ],
  [DataSourceKey.MYSQL]: [
    {
      label: 'Host',
      name: 'config.host',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'localhost',
    },
    {
      label: 'Port',
      name: 'config.port',
      type: FormFieldType.Number,
      required: true,
      placeholder: '3306',
    },
    {
      label: 'Database',
      name: 'config.database',
      type: FormFieldType.Text,
      required: true,
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
      label: 'SQL Query',
      name: 'config.query',
      type: FormFieldType.Textarea,
      required: false,
      placeholder: 'Leave empty to load all tables',
      tooltip: t('setting.mysqlQueryTip'),
    },
    {
      label: 'Content Columns',
      name: 'config.content_columns',
      type: FormFieldType.Text,
      required: false,
      placeholder: 'title,description,content',
      tooltip: t('setting.mysqlContentColumnsTip'),
    },
  ],
  [DataSourceKey.POSTGRESQL]: [
    {
      label: 'Host',
      name: 'config.host',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'localhost',
    },
    {
      label: 'Port',
      name: 'config.port',
      type: FormFieldType.Number,
      required: true,
      placeholder: '5432',
    },
    {
      label: 'Database',
      name: 'config.database',
      type: FormFieldType.Text,
      required: true,
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
      label: 'SQL Query',
      name: 'config.query',
      type: FormFieldType.Textarea,
      required: false,
      placeholder: 'Leave empty to load all tables',
      tooltip: t('setting.postgresqlQueryTip'),
    },
    {
      label: 'Content Columns',
      name: 'config.content_columns',
      type: FormFieldType.Text,
      required: false,
      placeholder: 'title,description,content',
      tooltip: t('setting.postgresqlContentColumnsTip'),
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
        region: '',
        authentication_method: 'access_key',
        aws_role_arn: '',
        endpoint_url: '',
        addressing_style: 'virtual',
      },
    },
  },
  [DataSourceKey.R2]: {
    name: '',
    source: DataSourceKey.R2,
    config: {
      bucket_name: '',
      credentials: {
        account_id: '',
        r2_access_key_id: '',
        r2_secret_access_key: '',
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
      page_id: '',
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
  [DataSourceKey.GOOGLE_CLOUD_STORAGE]: {
    name: '',
    source: DataSourceKey.GOOGLE_CLOUD_STORAGE,
    config: {
      bucket_name: '',
      credentials: {
        access_key_id: '',
        secret_access_key: '',
      },
    },
  },
  [DataSourceKey.OCI_STORAGE]: {
    name: '',
    source: DataSourceKey.OCI_STORAGE,
    config: {
      bucket_name: '',
      credentials: {
        namespace: '',
        region: '',
        access_key_id: '',
        secret_access_key: '',
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
  [DataSourceKey.AIRTABLE]: {
    name: '',
    source: DataSourceKey.AIRTABLE,
    config: {
      name: '',
      base_id: '',
      table_name_or_id: '',
      credentials: {
        airtable_access_token: '',
      },
    },
  },
  [DataSourceKey.GITLAB]: {
    name: '',
    source: DataSourceKey.GITLAB,
    config: {
      project_owner: '',
      project_name: '',
      gitlab_url: 'https://gitlab.com',
      include_mrs: true,
      include_issues: true,
      include_code_files: true,
      credentials: {
        gitlab_access_token: '',
      },
    },
  },
  [DataSourceKey.ASANA]: {
    name: '',
    source: DataSourceKey.ASANA,
    config: {
      name: '',
      asana_workspace_id: '',
      asana_project_ids: '',
      asana_team_id: '',
      credentials: {
        asana_api_token_secret: '',
      },
    },
  },
  [DataSourceKey.GITHUB]: {
    name: '',
    source: DataSourceKey.GITHUB,
    config: {
      repository_owner: '',
      repository_name: '',
      include_pull_requests: false,
      include_issues: false,
      credentials: {
        github_access_token: '',
      },
    },
  },
  [DataSourceKey.IMAP]: {
    name: '',
    source: DataSourceKey.IMAP,
    config: {
      name: '',
      imap_host: '',
      imap_port: 993,
      imap_mailbox: [],
      poll_range: 30,
      credentials: {
        imap_username: '',
        imap_password: '',
      },
    },
  },
  [DataSourceKey.BITBUCKET]: {
    name: '',
    source: DataSourceKey.BITBUCKET,
    config: {
      workspace: '',
      index_mode: 'workspace',
      repository_slugs: '',
      projects: '',
    },
    credentials: {
      bitbucket_api_token: '',
    },
  },
  [DataSourceKey.ZENDESK]: {
    name: '',
    source: DataSourceKey.ZENDESK,
    config: {
      name: '',
      zendesk_content_type: 'articles',
      credentials: {
        zendesk_subdomain: '',
        zendesk_email: '',
        zendesk_token: '',
      },
    },
  },
  [DataSourceKey.SEAFILE]: {
    name: '',
    source: DataSourceKey.SEAFILE,
    config: {
      seafile_url: '',
      include_shared: true,
      batch_size: 100,
      credentials: {
        seafile_token: '',
      },
    },
  },
  [DataSourceKey.MYSQL]: {
    name: '',
    source: DataSourceKey.MYSQL,
    config: {
      host: 'localhost',
      port: 3306,
      database: '',
      query: '',
      content_columns: '',
      metadata_columns: '',
      id_column: '',
      timestamp_column: '',
      credentials: {
        username: '',
        password: '',
      },
    },
  },
  [DataSourceKey.POSTGRESQL]: {
    name: '',
    source: DataSourceKey.POSTGRESQL,
    config: {
      host: 'localhost',
      port: 5432,
      database: '',
      query: '',
      content_columns: '',
      metadata_columns: '',
      id_column: '',
      timestamp_column: '',
      credentials: {
        username: '',
        password: '',
      },
    },
  },
};
