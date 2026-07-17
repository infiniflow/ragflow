import { FormFieldType } from '@/components/dynamic-form';

export const databricksConstant = () => [
  {
    label: 'Server Hostname',
    name: 'config.server_hostname',
    type: FormFieldType.Text,
    required: true,
    placeholder: 'dbc-xxxxxxxx.cloud.databricks.com',
  },
  {
    label: 'Authentication',
    name: 'config.auth_mode',
    type: FormFieldType.Segmented,
    options: [
      { label: 'Access Token (PAT)', value: 'pat' },
      { label: 'OAuth (M2M)', value: 'oauth' },
    ],
    defaultValue: 'pat',
  },
  {
    label: 'Access Token',
    name: 'config.credentials.access_token',
    type: FormFieldType.Password,
    shouldRender: (formValues: any) =>
      (formValues?.config?.auth_mode ?? 'pat') === 'pat',
    customValidate: (val: string, formValues: any) => {
      const mode = formValues?.config?.auth_mode ?? 'pat';
      if (mode === 'pat' && !val) {
        return 'Access Token is required';
      }
      return true;
    },
  },
  {
    label: 'Client ID',
    name: 'config.credentials.client_id',
    type: FormFieldType.Text,
    shouldRender: (formValues: any) =>
      formValues?.config?.auth_mode === 'oauth',
    customValidate: (val: string, formValues: any) => {
      if (
        formValues?.config?.auth_mode === 'oauth' &&
        !(val != null && String(val).trim())
      ) {
        return 'Client ID is required';
      }
      return true;
    },
  },
  {
    label: 'Client Secret',
    name: 'config.credentials.client_secret',
    type: FormFieldType.Password,
    shouldRender: (formValues: any) =>
      formValues?.config?.auth_mode === 'oauth',
    customValidate: (val: string, formValues: any) => {
      if (formValues?.config?.auth_mode === 'oauth' && !val) {
        return 'Client Secret is required';
      }
      return true;
    },
  },
  {
    label: 'SQL Warehouse HTTP Path',
    name: 'config.http_path',
    type: FormFieldType.Text,
    required: false,
    placeholder: '/sql/1.0/warehouses/abcdef1234567890',
    tooltip:
      'HTTP path of the SQL warehouse. Required when ingesting tables; leave empty for volumes-only sync.',
  },
  {
    label: 'Tables',
    name: 'config.tables',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'catalog.schema.table1, catalog.schema.table2',
    tooltip:
      'Comma-separated fully-qualified table names (catalog.schema.table) to ingest from the SQL warehouse.',
  },
  {
    label: 'Content Columns',
    name: 'config.content_columns',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'title,body',
    tooltip:
      'Comma-separated columns combined into each row’s document body. Required when ingesting tables.',
  },
  {
    label: 'Metadata Columns',
    name: 'config.metadata_columns',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'author,category',
  },
  {
    label: 'ID Column',
    name: 'config.id_column',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'id',
    tooltip:
      'Column used as the stable document ID. A content hash is used when omitted.',
  },
  {
    label: 'Watermark Column',
    name: 'config.timestamp_column',
    type: FormFieldType.Text,
    required: false,
    placeholder: 'ingested_at',
    tooltip:
      'Timestamp/ingest-timestamp column used for incremental table sync. Omit for a full re-scan each run.',
  },
  {
    label: 'Volume Paths',
    name: 'config.volume_paths',
    type: FormFieldType.Text,
    required: false,
    placeholder: '/Volumes/catalog/schema/volume/docs',
    tooltip:
      'Comma-separated Unity Catalog volume paths to ingest files (pdf/docx/md/txt) from. Incremental via file mtime.',
  },
];
