import { useEffect, useMemo, useState } from 'react';
import { useFormContext } from 'react-hook-form';

import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Input } from '@/components/ui/input';
import { Segmented } from '@/components/ui/segmented';
import { t } from 'i18next';

// UI-only auth modes for S3
// access_key: Access Key ID + Secret
// iam_role: only Role ARN
// assume_role: no input fields (uses environment role)
type AuthMode = 'access_key' | 'iam_role' | 'assume_role';
type BlobMode = 's3' | 's3_compatible';

const modeOptions = [
  { label: 'S3', value: 's3' },
  { label: 'S3 Compatible', value: 's3_compatible' },
];

const authOptions = [
  { label: 'Access Key', value: 'access_key' },
  { label: 'IAM Role', value: 'iam_role' },
  { label: 'Assume Role', value: 'assume_role' },
];

const addressingOptions = [
  { label: 'Virtual Hosted Style', value: 'virtual' },
  { label: 'Path Style', value: 'path' },
];

const deriveInitialAuthMode = (credentials: any): AuthMode => {
  const authMethod = credentials?.authentication_method;
  if (authMethod === 'iam_role') return 'iam_role';
  if (authMethod === 'assume_role') return 'assume_role';
  if (credentials?.aws_role_arn) return 'iam_role';
  if (credentials?.aws_access_key_id || credentials?.aws_secret_access_key)
    return 'access_key';
  return 'access_key';
};

const deriveInitialMode = (bucketType?: string): BlobMode =>
  bucketType === 's3_compatible' ? 's3_compatible' : 's3';

const BlobTokenField = () => {
  const form = useFormContext();
  const credentials = form.watch('config.credentials');
  const watchedBucketType = form.watch('config.bucket_type');

  const [mode, setMode] = useState<BlobMode>(
    deriveInitialMode(watchedBucketType),
  );
  const [authMode, setAuthMode] = useState<AuthMode>(() =>
    deriveInitialAuthMode(credentials),
  );

  // Keep bucket_type in sync with UI mode
  useEffect(() => {
    const nextMode = deriveInitialMode(watchedBucketType);
    setMode((prev) => (prev === nextMode ? prev : nextMode));
  }, [watchedBucketType]);

  useEffect(() => {
    form.setValue('config.bucket_type', mode, { shouldDirty: true });
    // Default addressing style for compatible mode
    if (
      mode === 's3_compatible' &&
      !form.getValues('config.credentials.addressing_style')
    ) {
      form.setValue('config.credentials.addressing_style', 'virtual', {
        shouldDirty: false,
      });
    }
    if (mode === 's3_compatible' && authMode !== 'access_key') {
      setAuthMode('access_key');
    }
    // Persist authentication_method for backend
    const nextAuthMethod: AuthMode =
      mode === 's3_compatible' ? 'access_key' : authMode;
    form.setValue('config.credentials.authentication_method', nextAuthMethod, {
      shouldDirty: true,
    });
    // Clear errors for fields that are not relevant in the current mode/auth selection
    const inactiveFields: string[] = [];
    if (mode === 's3_compatible') {
      inactiveFields.push('config.credentials.aws_role_arn');
    } else {
      if (authMode === 'iam_role') {
        inactiveFields.push('config.credentials.aws_access_key_id');
        inactiveFields.push('config.credentials.aws_secret_access_key');
      }
      if (authMode === 'assume_role') {
        inactiveFields.push('config.credentials.aws_access_key_id');
        inactiveFields.push('config.credentials.aws_secret_access_key');
        inactiveFields.push('config.credentials.aws_role_arn');
      }
    }
    if (inactiveFields.length) {
      form.clearErrors(inactiveFields as any);
    }
  }, [form, mode, authMode]);

  const isS3 = mode === 's3';
  const requiresAccessKey =
    authMode === 'access_key' || mode === 's3_compatible';
  const requiresRoleArn = isS3 && authMode === 'iam_role';

  // Help text for assume role (no inputs)
  const assumeRoleNote = useMemo(
    () => t('No credentials required. Uses the default environment role.'),
    [t],
  );

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <div className="text-sm text-text-secondary">Mode</div>
        <Segmented
          options={modeOptions}
          value={mode}
          onChange={(val) => setMode(val as BlobMode)}
          className="w-full"
          itemClassName="flex-1 justify-center"
        />
      </div>

      {isS3 && (
        <div className="flex flex-col gap-2">
          <div className="text-sm text-text-secondary">Authentication</div>
          <Segmented
            options={authOptions}
            value={authMode}
            onChange={(val) => setAuthMode(val as AuthMode)}
            className="w-full"
            itemClassName="flex-1 justify-center"
          />
        </div>
      )}

      {requiresAccessKey && (
        <RAGFlowFormItem
          name="config.credentials.aws_access_key_id"
          label="AWS Access Key ID"
          required={requiresAccessKey}
          rules={{
            validate: (val) =>
              requiresAccessKey
                ? Boolean(val) || 'Access Key ID is required'
                : true,
          }}
        >
          {(field) => (
            <Input {...field} placeholder="AKIA..." autoComplete="off" />
          )}
        </RAGFlowFormItem>
      )}

      {requiresAccessKey && (
        <RAGFlowFormItem
          name="config.credentials.aws_secret_access_key"
          label="AWS Secret Access Key"
          required={requiresAccessKey}
          rules={{
            validate: (val) =>
              requiresAccessKey
                ? Boolean(val) || 'Secret Access Key is required'
                : true,
          }}
        >
          {(field) => (
            <Input
              {...field}
              type="password"
              placeholder="****************"
              autoComplete="new-password"
            />
          )}
        </RAGFlowFormItem>
      )}

      {requiresRoleArn && (
        <RAGFlowFormItem
          name="config.credentials.aws_role_arn"
          label="Role ARN"
          required={requiresRoleArn}
          tooltip="The role will be assumed by the runtime environment."
          rules={{
            validate: (val) =>
              requiresRoleArn ? Boolean(val) || 'Role ARN is required' : true,
          }}
        >
          {(field) => (
            <Input
              {...field}
              placeholder="arn:aws:iam::123456789012:role/YourRole"
              autoComplete="off"
            />
          )}
        </RAGFlowFormItem>
      )}

      {isS3 && authMode === 'assume_role' && (
        <div className="text-sm text-text-secondary bg-bg-card border border-border-button rounded-md px-3 py-2">
          {assumeRoleNote}
        </div>
      )}

      {mode === 's3_compatible' && (
        <div className="flex flex-col gap-4">
          <RAGFlowFormItem
            name="config.credentials.addressing_style"
            label="Addressing Style"
            tooltip={t('setting.S3CompatibleAddressingStyleTip')}
            required={false}
          >
            {(field) => (
              <SelectWithSearch
                triggerClassName="!shrink"
                options={addressingOptions}
                value={field.value || 'virtual'}
                onChange={(val) => field.onChange(val)}
              />
            )}
          </RAGFlowFormItem>

          <RAGFlowFormItem
            name="config.credentials.endpoint_url"
            label="Endpoint URL"
            required={false}
            tooltip={t('setting.S3CompatibleEndpointUrlTip')}
          >
            {(field) => (
              <Input
                {...field}
                placeholder="https://fsn1.your-objectstorage.com"
                autoComplete="off"
              />
            )}
          </RAGFlowFormItem>
        </div>
      )}
    </div>
  );
};

export default BlobTokenField;
