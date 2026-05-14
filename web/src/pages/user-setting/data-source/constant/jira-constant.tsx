import { FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const jiraConstant = (t: TFunction) => [
  {
    label: 'Jira User Email',
    name: 'config.credentials.jira_user_email',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'you@example.com',
    tooltip: t('setting.jiraEmailTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud !== false) {
        return Boolean(val) || 'Jira User Email is required';
      }
      return true;
    },
  },
  {
    label: 'Jira Username',
    name: 'config.credentials.jira_username',
    type: FormFieldType.Text,
    required: true,
    tooltip: t('setting.jiraEmailTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud === false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud === false) {
        return Boolean(val) || 'Jira Username is required';
      }
      return true;
    },
  },
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
    label: 'Mode',
    name: 'config.is_cloud',
    type: FormFieldType.Segmented,
    options: [
      { label: 'Cloud', value: true },
      { label: 'Server', value: false },
    ],
    defaultValue: true,
  },
  {
    label: 'Jira API Token',
    name: 'config.credentials.jira_api_token',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.jiraTokenTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud !== false) {
        return Boolean(val) || 'Jira API Token is required';
      }
      return true;
    },
  },
  {
    label: 'Jira Password',
    name: 'config.credentials.jira_password',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.jiraPasswordTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud === false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud === false) {
        return Boolean(val) || 'Jira Password is required';
      }
      return true;
    },
  },
  {
    label: 'Use Scoped Token',
    name: 'config.scoped_token',
    type: FormFieldType.Checkbox,
    required: false,
    tooltip: t('setting.jiraScopedTokenTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
  },
];
