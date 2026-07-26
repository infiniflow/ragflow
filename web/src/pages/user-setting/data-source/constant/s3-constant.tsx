import { FilterFormField, FormFieldType } from '@/components/dynamic-form';
import { TFunction } from 'i18next';
import { BedrockRegionList } from '../../setting-model/constants';

const awsRegionOptions = BedrockRegionList.map((r) => ({
  label: r,
  value: r,
}));
export const S3Constant = (t: TFunction) => [
  {
    label: t('setting.dataSourceFieldBucketName'),
    name: 'config.bucket_name',
    type: FormFieldType.Text,
    required: true,
  },
  {
    label: t('setting.dataSourceFieldRegion'),
    name: 'config.credentials.region',
    type: FormFieldType.Select,
    required: false,
    options: awsRegionOptions,
    allowCustomValue: true,
    customValidate: (val: string, formValues: any) => {
      const credentials = formValues?.config?.credentials || {};
      const bucketType = formValues?.config?.bucket_type || 's3';
      const hasAccessKey = Boolean(
        credentials.aws_access_key_id || credentials.aws_secret_access_key,
      );
      if (bucketType === 's3' && hasAccessKey) {
        return Boolean(val) || t('setting.dataSourceS3RegionRequired');
      }
      return true;
    },
  },
  {
    label: t('setting.dataSourceFieldPrefix'),
    name: 'config.prefix',
    type: FormFieldType.Text,
    required: false,
    tooltip: t('setting.s3PrefixTip'),
  },

  {
    label: t('setting.dataSourceFieldMode'),
    name: 'config.bucket_type',
    type: FormFieldType.Segmented,
    options: [
      { label: 'S3', value: 's3' },
      {
        label: t('setting.dataSourceOptionS3Compatible'),
        value: 's3_compatible',
      },
    ],
  },
  {
    label: t('setting.dataSourceFieldAuthentication'),
    name: 'config.credentials.authentication_method',
    type: FormFieldType.Segmented,
    options: [
      { label: t('setting.dataSourceOptionAccessKey'), value: 'access_key' },
      { label: t('setting.dataSourceOptionIamRole'), value: 'iam_role' },
      { label: t('setting.dataSourceOptionAssumeRole'), value: 'assume_role' },
    ],
    shouldRender: (formValues: any) => {
      const bucketType = formValues?.config?.bucket_type;
      return bucketType === 's3';
    },
  },
  {
    name: 'config.credentials.aws_access_key_id',
    label: t('setting.dataSourceFieldAwsAccessKeyId'),
    type: FormFieldType.Text,
    customValidate: (val: string, formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      console.log('authMode', authMode, val);
      if (
        !val &&
        (authMode === 'access_key' || bucketType === 's3_compatible')
      ) {
        return t('setting.dataSourceValidationFieldRequired', {
          label: t('setting.dataSourceFieldAwsAccessKeyId'),
        });
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      return authMode === 'access_key' || bucketType === 's3_compatible';
    },
  },
  {
    name: 'config.credentials.aws_secret_access_key',
    label: t('setting.dataSourceFieldAwsSecretAccessKey'),
    type: FormFieldType.Password,
    customValidate: (val: string, formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      if (authMode === 'access_key' || bucketType === 's3_compatible') {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldAwsSecretAccessKey'),
          })
        );
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      return authMode === 'access_key' || bucketType === 's3_compatible';
    },
  },
  {
    name: 'config.credentials.aws_role_arn',
    label: t('setting.dataSourceFieldRoleArn'),
    tooltip: t('setting.dataSourceS3RoleArnTip'),
    type: FormFieldType.Text,
    placeholder: 'arn:aws:iam::123456789012:role/YourRole',
    customValidate: (val: string, formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      if (authMode === 'iam_role' || bucketType === 's3') {
        return (
          Boolean(val) ||
          t('setting.dataSourceValidationFieldRequired', {
            label: t('setting.dataSourceFieldAwsSecretAccessKey'),
          })
        );
      }
      return true;
    },
    shouldRender: (formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      return authMode === 'iam_role' && bucketType === 's3';
    },
  },
  {
    name: FilterFormField + '.tip',
    label: ' ',
    type: FormFieldType.Custom,
    shouldRender: (formValues: any) => {
      const authMode = formValues?.config?.credentials?.authentication_method;
      const bucketType = formValues?.config?.bucket_type;
      return authMode === 'assume_role' && bucketType === 's3';
    },
    render: () => (
      <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
        {t('setting.dataSourceS3AssumeRoleTip')}
      </div>
    ),
  },
  {
    name: 'config.credentials.addressing_style',
    label: t('setting.dataSourceFieldAddressingStyle'),
    tooltip: t('setting.S3CompatibleAddressingStyleTip'),
    required: false,
    type: FormFieldType.Select,
    defaultValue: 'virtual',
    options: [
      {
        label: t('setting.dataSourceOptionVirtualHostedStyle'),
        value: 'virtual',
      },
      { label: t('setting.dataSourceOptionPathStyle'), value: 'path' },
    ],
    shouldRender: (formValues: any) => {
      // const authMode = formValues?.config?.authMode;
      const bucketType = formValues?.config?.bucket_type;
      return bucketType === 's3_compatible';
    },
  },
  {
    name: 'config.credentials.endpoint_url',
    label: t('setting.dataSourceFieldEndpointUrl'),
    tooltip: t('setting.S3CompatibleEndpointUrlTip'),
    placeholder: 'https://fsn1.your-objectstorage.com',
    required: false,
    type: FormFieldType.Text,
    shouldRender: (formValues: any) => {
      const bucketType = formValues?.config?.bucket_type;
      return bucketType === 's3_compatible';
    },
  },
  // {
  //   label: 'Credentials',
  //   name: 'config.credentials.__blob_token',
  //   type: FormFieldType.Custom,
  //   hideLabel: true,
  //   required: false,
  //   render: () => <BlobTokenField />,
  // },
];
