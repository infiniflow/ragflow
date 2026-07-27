import { FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';

export const jiraConstant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldJiraUserEmail'),
    name: 'config.credentials.jira_user_email',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'you@example.com',
    tooltip: t('setting.jiraEmailTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud !== false) {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldJiraUserEmail'),
          })
        );
      }
      return true;
    },
  },
  {
    label: t('setting.dataSourceFieldJiraUsername'),
    name: 'config.credentials.jira_username',
    type: FormFieldType.Text,
    required: true,
    tooltip: t('setting.jiraEmailTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud === false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud === false) {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldJiraUsername'),
          })
        );
      }
      return true;
    },
  },
  {
    label: t('setting.dataSourceFieldJiraBaseUrl'),
    name: 'config.base_url',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'https://your-domain.atlassian.net',
    tooltip: t('setting.jiraBaseUrlTip'),
  },
  {
    label: t('setting.dataSourceFieldProjectKey'),
    name: 'config.project_key',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'RAGFlow',
    tooltip: t('setting.jiraProjectKeyTip'),
  },
  {
    label: t('setting.dataSourceFieldCustomJql'),
    name: 'config.jql_query',
    type: FormFieldType.Textarea,
    required: false,
    placeholder: 'project = RAG AND updated >= -7d',
    tooltip: t('setting.jiraJqlTip'),
  },
  {
    label: t('setting.dataSourceFieldBatchSize'),
    name: 'config.batch_size',
    type: FormFieldType.Number,
    required: false,
    tooltip: t('setting.jiraBatchSizeTip'),
  },
  {
    label: t('setting.dataSourceFieldAttachmentSizeLimit'),
    name: 'config.attachment_size_limit',
    type: FormFieldType.Number,
    required: false,
    defaultValue: 10 * 1024 * 1024,
    tooltip: t('setting.jiraAttachmentSizeTip'),
  },
  {
    label: t('setting.dataSourceFieldLabelsToSkip'),
    name: 'config.labels_to_skip',
    type: FormFieldType.Tag,
    required: false,
    tooltip: t('setting.jiraLabelsTip'),
  },
  {
    label: t('setting.dataSourceFieldCommentEmailBlacklist'),
    name: 'config.comment_email_blacklist',
    type: FormFieldType.Tag,
    required: false,
    tooltip: t('setting.jiraBlacklistTip'),
  },
  {
    label: t('setting.dataSourceFieldIncludeComments'),
    name: 'config.include_comments',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: true,
    tooltip: t('setting.jiraCommentsTip'),
  },
  {
    label: t('setting.dataSourceFieldIncludeAttachments'),
    name: 'config.include_attachments',
    type: FormFieldType.Checkbox,
    required: false,
    defaultValue: false,
    tooltip: t('setting.jiraAttachmentsTip'),
  },
  {
    label: t('setting.dataSourceFieldMode'),
    name: 'config.is_cloud',
    type: FormFieldType.Segmented,
    options: [
      { label: t('setting.dataSourceOptionCloud'), value: true },
      { label: t('setting.dataSourceOptionServer'), value: false },
    ],
    defaultValue: true,
  },
  {
    label: t('setting.dataSourceFieldJiraApiToken'),
    name: 'config.credentials.jira_api_token',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.jiraTokenTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud !== false) {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldJiraApiToken'),
          })
        );
      }
      return true;
    },
  },
  {
    label: t('setting.dataSourceFieldJiraPassword'),
    name: 'config.credentials.jira_password',
    type: FormFieldType.Password,
    required: false,
    tooltip: t('setting.jiraPasswordTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud === false,
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.is_cloud === false) {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldJiraPassword'),
          })
        );
      }
      return true;
    },
  },
  {
    label: t('setting.dataSourceFieldUseScopedToken'),
    name: 'config.scoped_token',
    type: FormFieldType.Checkbox,
    required: false,
    tooltip: t('setting.jiraScopedTokenTip'),
    shouldRender: (formValues: any) => formValues?.config?.is_cloud !== false,
  },
];
