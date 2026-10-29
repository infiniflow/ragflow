/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import message from '@/components/ui/message';
import { Segmented } from '@/components/ui/segmented';
import { useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import {
  useDeleteProviderInstance,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import type { IProviderInstance } from '@/interfaces/database/llm';
import type { IModelInfo } from '@/interfaces/request/llm';
import { zodResolver } from '@hookform/resolvers/zod';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BedrockRegionList } from '../constants';
import { VerifyResult } from '../hooks';
import { splitProviderPayload } from '../payload-utils';
import {
  ProviderInstanceCardProps,
  ProviderInstanceCardRef,
} from './interface';
import {
  applySavedCredentialEdits,
  SAVED_CREDENTIAL_MASK,
  unwrapApiKey,
} from './hooks';
import {
  SavedProviderConfigFieldSet,
  hasUnsavedProviderCredential,
  pickSavedProviderConfigOverrides,
} from './saved-provider-config';
import { SavedCredentialInput } from './saved-credential-input';
import { ModelsSection } from './models-section';
import VerifyButton from './verify-button';

type AuthMode =
  | 'access_key_secret'
  | 'iam_role'
  | 'assume_role'
  | 'bedrock_api_key';

type BedrockFormValues = {
  auth_mode: AuthMode;
  bedrock_ak?: string;
  bedrock_sk?: string;
  aws_role_arn?: string;
  bedrock_api_key?: string;
  bedrock_region: string;
  llm_name: string;
  max_tokens: number;
  model_type: ('chat' | 'embedding')[];
};

interface BedrockInstanceCardProps {
  providerName: string;
  instance: IProviderInstance;
  isDraft?: boolean;
  onDelete?: () => void;
  defaultOpen?: boolean;
}

/**
 * Inline instance card for AWS Bedrock. Renders Bedrock-specific fields
 * (auth_mode segmented, credentials, region, and manual model settings)
 * directly instead of going through the generic DynamicForm path. API key
 * drafts discover models and persist them with the instance.
 * The parent page's top Save button drives persistence through the imperative
 * ref API.
 */
export const BedrockInstanceCard = forwardRef<
  ProviderInstanceCardRef,
  BedrockInstanceCardProps
>(function BedrockInstanceCard(
  { providerName, instance, isDraft = false, onDelete, defaultOpen = false },
  ref,
) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const [open, setOpen] = useState(isDraft || defaultOpen);
  const [draftName, setDraftName] = useState('');
  const modelInfoRef = useRef<IModelInfo[]>([]);
  const modelsLoadedRef = useRef(false);

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
    }
  }, [providerName, isDraft]);

  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instance.id,
    );

  const FormSchema = useMemo(
    () =>
      z
        .object({
          auth_mode: z
            .enum([
              'access_key_secret',
              'iam_role',
              'assume_role',
              'bedrock_api_key',
            ])
            .default('access_key_secret'),
          bedrock_ak: z.string().optional(),
          bedrock_sk: z.string().optional(),
          aws_role_arn: z.string().optional(),
          bedrock_api_key: z.string().optional(),
          bedrock_region: z
            .string()
            .min(1, { message: tSetting('bedrockRegionMessage') }),
          llm_name: z.string(),
          max_tokens: z.number({
            required_error: tSetting('maxTokensMessage'),
            invalid_type_error: tSetting('maxTokensInvalidMessage'),
          }),
          model_type: z.array(z.enum(['chat', 'embedding'])),
        })
        .superRefine((data, ctx) => {
          if (isDraft && data.auth_mode !== 'bedrock_api_key') {
            if (!data.llm_name.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockModelNameMessage'),
                path: ['llm_name'],
              });
            }
            if (data.model_type.length === 0) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('modelTypeMessage'),
                path: ['model_type'],
              });
            }
            if (data.max_tokens < 0) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('maxTokensMinMessage'),
                path: ['max_tokens'],
              });
            }
          }
          const savedConfig = instanceDetails?.api_key_config ?? {};
          const storedAuthMode = savedConfig.auth_mode;
          const credentialStatus = instanceDetails?.credential_status ?? {};
          const mayKeepStoredCredentials =
            !isDraft && data.auth_mode === storedAuthMode;
          const hasStoredBedrockAccessKeys =
            mayKeepStoredCredentials &&
            credentialStatus.bedrock_ak === true &&
            credentialStatus.bedrock_sk === true;
          const removingStoredAccessKeys =
            hasStoredBedrockAccessKeys &&
            data.bedrock_ak === '' &&
            data.bedrock_sk === '';
          if (
            data.auth_mode === 'access_key_secret' &&
            !removingStoredAccessKeys
          ) {
            if (!data.bedrock_ak?.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockAKMessage'),
                path: ['bedrock_ak'],
              });
            }
            if (!data.bedrock_sk?.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockSKMessage'),
                path: ['bedrock_sk'],
              });
            }
          }
          if (data.auth_mode === 'iam_role') {
            const canKeepStoredRoleArn =
              mayKeepStoredCredentials &&
              credentialStatus.aws_role_arn === true &&
              data.aws_role_arn === SAVED_CREDENTIAL_MASK;
            const removingStoredRoleArn =
              mayKeepStoredCredentials &&
              credentialStatus.aws_role_arn === true &&
              data.aws_role_arn === '';
            if (
              !data.aws_role_arn?.trim() &&
              !canKeepStoredRoleArn &&
              !removingStoredRoleArn
            ) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('awsRoleArnMessage'),
                path: ['aws_role_arn'],
              });
            }
          }
          if (
            data.auth_mode === 'bedrock_api_key' &&
            !data.bedrock_api_key?.trim() &&
            !(
              mayKeepStoredCredentials &&
              credentialStatus.bedrock_api_key === true
            )
          ) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: tSetting('apiKeyMessage'),
              path: ['bedrock_api_key'],
            });
          }
        }),
    [instanceDetails, isDraft, tSetting],
  );

  // Lazily fetch full instance details only when the card is open.
  // Collapsed cards never hit /providers/<name>/instances/<instance_name>;
  // expanding one triggers a fresh refetch.
  useEffect(() => {
    if (!isDraft && open && providerName && instance.instance_name) {
      refetchInstanceDetails();
    }
  }, [
    isDraft,
    open,
    providerName,
    instance.instance_name,
    refetchInstanceDetails,
  ]);

  const initialValues = useMemo<BedrockFormValues>(() => {
    const merged = { ...instance, ...(instanceDetails ?? {}) } as any;
    const { nested: apiKey } = unwrapApiKey(merged.api_key_config);
    const credentialStatus = merged.credential_status ?? {};
    return {
      auth_mode: (apiKey.auth_mode as AuthMode) ?? 'access_key_secret',
      bedrock_ak:
        credentialStatus.bedrock_ak === true
          ? SAVED_CREDENTIAL_MASK
          : (apiKey.bedrock_ak ?? ''),
      bedrock_sk:
        credentialStatus.bedrock_sk === true
          ? SAVED_CREDENTIAL_MASK
          : (apiKey.bedrock_sk ?? ''),
      aws_role_arn:
        credentialStatus.aws_role_arn === true
          ? SAVED_CREDENTIAL_MASK
          : (apiKey.aws_role_arn ?? ''),
      bedrock_api_key:
        credentialStatus.bedrock_api_key === true
          ? SAVED_CREDENTIAL_MASK
          : (apiKey.bedrock_api_key ?? ''),
      bedrock_region:
        apiKey.bedrock_region ??
        (merged.region && merged.region !== 'default' ? merged.region : ''),
      llm_name: '',
      max_tokens: 8192,
      model_type: ['chat'],
    };
  }, [instance, instanceDetails]);

  const form = useForm<BedrockFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues,
  });

  useLayoutEffect(() => {
    // Reset form when initial values change (e.g. instance details load).
    // ModelsSection reads the form in a passive effect, so credentials must
    // be restored before child effects start model discovery.
    form.reset(initialValues, { keepDirtyValues: true });
    // oxlint-disable-next-line react/exhaustive-deps
  }, [initialValues]);

  const authMode = useWatch({ control: form.control, name: 'auth_mode' });
  const storedCredentialStatus = useMemo(
    () => instanceDetails?.credential_status ?? {},
    [instanceDetails?.credential_status],
  );
  const canKeepStoredCredentials =
    !isDraft && authMode === instanceDetails?.api_key_config?.auth_mode;
  const canKeepStoredBedrockAccessKeys =
    canKeepStoredCredentials &&
    storedCredentialStatus.bedrock_ak === true &&
    storedCredentialStatus.bedrock_sk === true;
  const canKeepStoredBedrockAPIKey =
    canKeepStoredCredentials && storedCredentialStatus.bedrock_api_key === true;

  const regionOptions = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: tSetting(x) })),
    [tSetting],
  );

  // Build a Bedrock-shaped payload for both submit and verify flows.
  const buildPayload = useCallback(
    (values: BedrockFormValues, instanceName: string, forDraft: boolean) => {
      const cleaned: Record<string, any> = { ...values };
      const fieldsByMode: Record<AuthMode, string[]> = {
        access_key_secret: ['bedrock_ak', 'bedrock_sk'],
        iam_role: ['aws_role_arn'],
        assume_role: [],
        bedrock_api_key: ['bedrock_api_key'],
      };
      (Object.keys(fieldsByMode) as AuthMode[]).forEach((mode) => {
        if (mode !== values.auth_mode) {
          fieldsByMode[mode].forEach((f) => {
            delete cleaned[f];
          });
        }
      });

      const flat = {
        ...cleaned,
        instance_name: instanceName,
        llm_factory: providerName,
        max_tokens: values.max_tokens,
        model_type: values.model_type,
      };
      const { instancePayload, modelPayload } = splitProviderPayload(flat);
      const credentialStatus = instanceDetails?.credential_status ?? {};
      const payload: Record<string, any> = {
        instance_name: instanceName,
        api_key: applySavedCredentialEdits(
          instancePayload.api_key,
          values,
          credentialStatus,
        ),
        base_url: instancePayload.base_url,
        region: instancePayload.region,
      };
      if (forDraft) {
        payload.llm_factory = providerName;
      } else {
        const storedAuthMode = instanceDetails?.api_key_config?.auth_mode;
        if (storedAuthMode && storedAuthMode !== values.auth_mode) {
          payload.clear_api_key = true;
        }
      }
      if (forDraft && values.auth_mode !== 'bedrock_api_key') {
        payload.max_tokens = modelPayload.max_tokens;
        payload.model_info = [modelPayload];
      } else {
        payload.model_info = modelInfoRef.current;
      }
      return payload;
    },
    [
      instanceDetails?.api_key_config,
      instanceDetails?.credential_status,
      providerName,
    ],
  );

  const transformModelCredentials = useCallback(
    (values: Record<string, any>) => {
      const payload = buildPayload(
        values as BedrockFormValues,
        draftName.trim() || instance.instance_name,
        false,
      );
      return {
        apiKey: payload.api_key,
        baseUrl: payload.base_url,
        region: payload.region,
      };
    },
    [buildPayload, draftName, instance.instance_name],
  );

  const getModelFormValues = useCallback(() => {
    const values = form.getValues();
    const credentials = transformModelCredentials(values);
    return {
      ...values,
      api_key: credentials.apiKey,
      base_url: credentials.baseUrl,
    };
  }, [form, transformModelCredentials]);

  const { verifyProviderConnection } = useVerifyProviderConnection();
  const handleVerify = useCallback(async () => {
    const isValid = await form.trigger();
    if (!isValid) {
      return {
        isValid: false,
        logs: tSetting('bedrockRegionMessage'),
      } as VerifyResult;
    }
    const values = form.getValues();
    const payload = buildPayload(
      values,
      draftName.trim() || instance.instance_name,
      isDraft,
    );
    const currentValues = values as Record<string, any>;
    const credentialChanged = Object.entries(storedCredentialStatus).some(
      ([field, configured]) =>
        configured &&
        !SavedProviderConfigFieldSet.has(field) &&
        currentValues[field] !== undefined &&
        currentValues[field] !== SAVED_CREDENTIAL_MASK,
    );
    const useSavedCredentials =
      !isDraft &&
      !payload.clear_api_key &&
      !credentialChanged &&
      !hasUnsavedProviderCredential(payload.api_key);
    const canOverrideSavedCredentials =
      useSavedCredentials && instanceDetails?.is_owner === true;
    const savedConfigOverrides = canOverrideSavedCredentials
      ? pickSavedProviderConfigOverrides(payload.api_key)
      : {};
    const ret = await verifyProviderConnection({
      provider_name: providerName,
      ...(!isDraft ? { instance_id: instance.id } : {}),
      ...(payload.clear_api_key ? { clear_api_key: true } : {}),
      ...(useSavedCredentials
        ? {
            use_saved_credentials: true,
            ...(Object.keys(savedConfigOverrides).length > 0
              ? { api_key_config: savedConfigOverrides }
              : {}),
            ...(canOverrideSavedCredentials
              ? { base_url: payload.base_url, region: payload.region }
              : {}),
          }
        : {
            api_key: JSON.stringify(payload.api_key),
            base_url: payload.base_url,
            region: payload.region,
          }),
      model_info: payload.model_info,
    });
    return {
      isValid: ret.code === 0,
      logs: ret.message,
    } as VerifyResult;
  }, [
    form,
    providerName,
    buildPayload,
    draftName,
    instance.instance_name,
    instance.id,
    instanceDetails?.is_owner,
    storedCredentialStatus,
    verifyProviderConnection,
    tSetting,
    isDraft,
  ]);

  const { deleteProviderInstance } = useDeleteProviderInstance();
  const handleDelete = useCallback(async () => {
    if (isDraft) {
      onDelete?.();
    } else {
      await deleteProviderInstance({
        provider_name: providerName,
        instances: [instance.instance_name],
      });
    }
  }, [
    isDraft,
    providerName,
    instance.instance_name,
    deleteProviderInstance,
    onDelete,
  ]);

  // ── Dirty tracking (no auto-save) ────────────────────────────────
  // Baseline signature mirrors the persisted state so `getSavePayload`
  // can skip redundant saves. For drafts the baseline stays empty
  // (drafts are always dirty once a name is typed).
  const baselinePayloadRef = useRef<string>('');
  const draftNameRef = useRef(draftName);
  useEffect(() => {
    draftNameRef.current = draftName;
  });

  useEffect(() => {
    if (isDraft) {
      baselinePayloadRef.current = '';
      return;
    }
    if (!instanceDetails && !instance.id) return;
    const baselineValues = initialValues;
    const baseline = buildPayload(
      baselineValues,
      instance.instance_name,
      false,
    );
    const finalBaseline = {
      ...baseline,
      provider_name: providerName,
      id: instanceDetails?.id || instance.id,
      verify: false,
    };
    baselinePayloadRef.current = JSON.stringify(finalBaseline);
  }, [
    isDraft,
    initialValues,
    buildPayload,
    providerName,
    instance.instance_name,
    instance.id,
    instanceDetails,
  ]);

  const getSavePayload = useCallback(() => {
    const trimmed = draftNameRef.current.trim();
    if (isDraft) {
      if (!trimmed) return null;
      const values = form.getValues();
      const payload = buildPayload(values, trimmed, true);
      return {
        payload,
        instanceName: trimmed,
        isDraft: true,
        // Bedrock drafts use the add endpoint (no id).
        apiKind: 'add' as const,
      };
    }
    const values = form.getValues();
    const payload = buildPayload(values, instance.instance_name, false);
    const finalPayload = {
      ...payload,
      provider_name: providerName,
      id: instanceDetails?.id || instance.id,
      verify: false,
    };
    const sig = JSON.stringify(finalPayload);
    if (sig === baselinePayloadRef.current) return null;
    return {
      payload: finalPayload,
      instanceName: instance.instance_name,
      isDraft: false,
      apiKind: 'update' as const,
    };
  }, [
    isDraft,
    form,
    buildPayload,
    providerName,
    instance.instance_name,
    instance.id,
    instanceDetails,
  ]);

  const markSaved = useCallback(() => {
    const result = getSavePayload();
    if (result) {
      baselinePayloadRef.current = JSON.stringify(result.payload);
    }
  }, [getSavePayload]);

  const markModelsEdited = useCallback(() => {
    if (isDraft || !baselinePayloadRef.current) return;
    try {
      const baseline = JSON.parse(baselinePayloadRef.current) as Record<
        string,
        any
      >;
      baseline.model_info = modelInfoRef.current;
      baselinePayloadRef.current = JSON.stringify(baseline);
    } catch {
      // ignore parse errors - baseline will be re-seeded on details reload
    }
  }, [isDraft]);

  useImperativeHandle(
    ref,
    () => ({
      validate: async () => {
        if (isDraft && !draftNameRef.current.trim()) return false;
        if (!isDraft && !modelsLoadedRef.current) {
          message.error(tSetting('selectModelBeforeSave'));
          return false;
        }
        if (
          isDraft &&
          authMode === 'bedrock_api_key' &&
          modelInfoRef.current.length === 0
        ) {
          message.error(tSetting('selectModelBeforeSave'));
          return false;
        }
        const isValid = await form.trigger();
        return !!isValid;
      },
      getSavePayload,
      markSaved,
    }),
    [isDraft, authMode, form, getSavePayload, markSaved, tSetting],
  );

  // ──────────────── Field group rendered in both modes ────────────────
  const renderFields = () => (
    <Form {...form}>
      <form className="space-y-6" onSubmit={(e) => e.preventDefault()}>
        {isDraft && authMode !== 'bedrock_api_key' && (
          <>
            <RAGFlowFormItem
              name="model_type"
              label={tSetting('modelType')}
              required
            >
              {(field) => (
                <MultiSelect
                  options={buildModelTypeOptions(['chat', 'embedding'])}
                  placeholder={tSetting('modelTypeMessage')}
                  onValueChange={field.onChange}
                  defaultValue={field.value}
                  variant="inverted"
                  maxCount={100}
                />
              )}
            </RAGFlowFormItem>

            <RAGFlowFormItem
              name="llm_name"
              label={tSetting('modelName')}
              required
            >
              <Input placeholder={tSetting('bedrockModelNameMessage')} />
            </RAGFlowFormItem>
          </>
        )}

        <div>
          <RAGFlowFormItem name="auth_mode">
            {(field) => (
              <Segmented
                value={field.value}
                onChange={(value) => {
                  if (isDraft && value !== field.value) {
                    modelInfoRef.current = [];
                  }
                  if (value !== 'access_key_secret') {
                    form.setValue('bedrock_ak', '');
                    form.setValue('bedrock_sk', '');
                  }
                  if (value !== 'iam_role') {
                    form.setValue('aws_role_arn', '');
                  }
                  if (value !== 'bedrock_api_key') {
                    form.setValue('bedrock_api_key', '');
                  }
                  field.onChange(value);
                }}
                options={[
                  {
                    label: tSetting('awsAuthModeAccessKeySecret'),
                    value: 'access_key_secret',
                  },
                  { label: tSetting('awsAuthModeIamRole'), value: 'iam_role' },
                  {
                    label: tSetting('awsAuthModeAssumeRole'),
                    value: 'assume_role',
                  },
                  {
                    label: tSetting('apiKey'),
                    value: 'bedrock_api_key',
                  },
                ]}
              />
            )}
          </RAGFlowFormItem>
        </div>

        {authMode === 'access_key_secret' && (
          <>
            <RAGFlowFormItem
              name="bedrock_ak"
              label={tSetting('awsAccessKeyId')}
              required={!canKeepStoredBedrockAccessKeys}
            >
              {(field) => (
                <SavedCredentialInput
                  {...field}
                  maskedValue={instanceDetails?.credential_masks?.bedrock_ak}
                  placeholder={tSetting(
                    storedCredentialStatus.bedrock_ak === true
                      ? 'credentialClearToRemove'
                      : 'bedrockAKMessage',
                  )}
                  autoComplete="new-password"
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="bedrock_sk"
              label={tSetting('awsSecretAccessKey')}
              required={!canKeepStoredBedrockAccessKeys}
            >
              {(field) => (
                <SavedCredentialInput
                  {...field}
                  maskedValue={instanceDetails?.credential_masks?.bedrock_sk}
                  placeholder={tSetting(
                    storedCredentialStatus.bedrock_sk === true
                      ? 'credentialClearToRemove'
                      : 'bedrockSKMessage',
                  )}
                  autoComplete="new-password"
                />
              )}
            </RAGFlowFormItem>
          </>
        )}

        {authMode === 'iam_role' && (
          <RAGFlowFormItem
            name="aws_role_arn"
            label={tSetting('awsRoleArn')}
            required={
              !canKeepStoredCredentials ||
              storedCredentialStatus.aws_role_arn !== true
            }
          >
            {(field) => (
              <SavedCredentialInput
                {...field}
                maskedValue={instanceDetails?.credential_masks?.aws_role_arn}
                autoComplete="new-password"
                placeholder={tSetting(
                  storedCredentialStatus.aws_role_arn === true
                    ? 'credentialClearToRemove'
                    : 'awsRoleArnMessage',
                )}
              />
            )}
          </RAGFlowFormItem>
        )}

        {authMode === 'bedrock_api_key' && (
          <>
            <RAGFlowFormItem
              name="bedrock_api_key"
              label={tSetting('apiKey')}
              required={!canKeepStoredBedrockAPIKey}
            >
              {(field) => (
                <SavedCredentialInput
                  {...field}
                  maskedValue={
                    instanceDetails?.credential_masks?.bedrock_api_key
                  }
                  placeholder={tSetting(
                    storedCredentialStatus.bedrock_api_key === true
                      ? 'credentialClearToRemove'
                      : 'apiKeyMessage',
                  )}
                  autoComplete="new-password"
                />
              )}
            </RAGFlowFormItem>
          </>
        )}

        {authMode === 'assume_role' && (
          <div className="text-sm text-text-secondary">
            {tSetting('awsAssumeRoleTip')}
          </div>
        )}

        <RAGFlowFormItem
          name="bedrock_region"
          label={tSetting('bedrockRegion')}
          required
        >
          {(field) => (
            <SelectWithSearch
              value={field.value}
              onChange={field.onChange}
              options={regionOptions}
              placeholder={tSetting('bedrockRegionMessage')}
              allowClear
            />
          )}
        </RAGFlowFormItem>

        {isDraft && authMode !== 'bedrock_api_key' && (
          <RAGFlowFormItem
            name="max_tokens"
            label={tSetting('maxTokens')}
            required
          >
            {(field) => (
              <Input
                type="number"
                placeholder={tSetting('maxTokensTip')}
                value={field.value}
                onChange={(e) => field.onChange(Number(e.target.value))}
              />
            )}
          </RAGFlowFormItem>
        )}
      </form>

      {/* VerifyButton lives inside <Form> (FormProvider) so its
          internal useFormContext() resolves the form instance.
          Rendered outside <form> so it never triggers submission. */}
      {authMode !== 'bedrock_api_key' && (
        <div className="pt-3">
          <VerifyButton onVerify={handleVerify} isAbsolute={false} />
        </div>
      )}
    </Form>
  );

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {isDraft ? (
        <div className="px-2 py-3 flex flex-col gap-4">
          <div
            className="flex flex-col gap-1.5"
            data-testid="instance-name-section"
          >
            <label
              htmlFor="instance-name-input"
              className="text-sm font-medium text-text-primary"
            >
              <span className="text-destructive mr-0.5">*</span>
              {tSetting('instanceName')}
            </label>
            <div className="flex items-center">
              <Input
                id="instance-name-input"
                value={draftName}
                onChange={(e) => setDraftName(e.target.value)}
                placeholder={tSetting('instanceNamePlaceholder')}
                className="flex-1"
                data-testid="instance-name-input"
              />
              <ConfirmDeleteDialog onOk={handleDelete}>
                <Button
                  variant="delete"
                  size="icon-sm"
                  className="ml-2 shrink-0"
                  aria-label={tSetting('deleteInstance')}
                  data-testid="draft-delete"
                >
                  <Trash2 className="size-4" />
                </Button>
              </ConfirmDeleteDialog>
            </div>
          </div>

          {renderFields()}

          <div className="pt-3">
            <ModelsSection
              key={authMode}
              providerName={providerName}
              instanceName={instance.instance_name || '__draft__'}
              instance={instance}
              hideActions={false}
              hideIfEmpty={false}
              getFormValues={getModelFormValues}
              verifyTransform={transformModelCredentials}
              onInstanceModelsChange={(info) => {
                modelInfoRef.current = info;
              }}
            />
          </div>
        </div>
      ) : (
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger asChild>
            <div className="flex items-center gap-1 w-full mb-5">
              <div
                className="group flex items-center flex-1 gap-2 px-2 mx-2 py-1 cursor-pointer bg-bg-input rounded-md"
                data-testid="instance-name-row"
              >
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={
                    open ? t('setting.hideModels') : t('setting.showMoreModels')
                  }
                  data-testid="instance-collapse"
                >
                  {open ? (
                    <ListChevronsDownUp className="size-4" />
                  ) : (
                    <ListChevronsUpDown className="size-4" />
                  )}
                </Button>
                <span
                  className="text-sm font-medium"
                  data-testid="instance-name-static"
                >
                  {draftName || instance.instance_name}
                </span>
              </div>
              <ConfirmDeleteDialog onOk={handleDelete}>
                <Button
                  variant="delete"
                  size="icon-sm"
                  aria-label={tSetting('deleteInstance')}
                  data-testid="instance-delete"
                  onClick={(e: React.MouseEvent) => e.stopPropagation()}
                >
                  <Trash2 className="size-4" />
                </Button>
              </ConfirmDeleteDialog>
            </div>
          </CollapsibleTrigger>
          <CollapsibleContent
            forceMount
            className="data-[state=closed]:hidden overflow-hidden"
          >
            <div className="px-2 pb-4 flex flex-col gap-4">
              {renderFields()}

              {instanceDetails && (
                <div className="pt-3">
                  <ModelsSection
                    providerName={providerName}
                    instanceName={instance.instance_name || '__draft__'}
                    instance={{ ...instance, ...(instanceDetails ?? {}) }}
                    hideActions={false}
                    hideIfEmpty={false}
                    instanceDetailsLoaded
                    getFormValues={getModelFormValues}
                    verifyTransform={transformModelCredentials}
                    onInstanceModelsChange={(info) => {
                      modelInfoRef.current = info;
                    }}
                    onInstanceModelsEdited={markModelsEdited}
                    onInstanceModelsStatusChange={(ready) => {
                      modelsLoadedRef.current = ready;
                    }}
                  />
                </div>
              )}
            </div>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
});

export default BedrockInstanceCard;

// Ensure the component is usable with the same props shape as the
// generic card (keeps the dispatch in provider-instance-card.tsx happy
// when forwarding props + ref).
export type { ProviderInstanceCardProps };
