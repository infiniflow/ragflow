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
import message from '@/components/ui/message';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Segmented } from '@/components/ui/segmented';
import { useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import {
  useDeleteProviderInstance,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import {
  IAddProviderInstanceRequestBody,
  IModelInfo,
} from '@/interfaces/request/llm';
import { zodResolver } from '@hookform/resolvers/zod';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
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
import { parseApiKeyAsObject } from '../provider-schema/field-config/utils';
import {
  getBedrockCatalogCredentialScope,
  getBedrockModelListRequest,
  shouldResetBedrockForm,
} from './bedrock-instance-utils';
import type {
  AuthMode,
  BedrockEndpointType,
  BedrockFormValues,
} from './bedrock-instance-utils';
import {
  ProviderInstanceCardProps,
  ProviderInstanceCardRef,
} from './interface';
import { ModelsSection } from './models-section';
import VerifyButton from './verify-button';

interface BedrockInstanceCardProps {
  providerName: string;
  instance: IProviderInstance;
  isDraft?: boolean;
  onDelete?: () => void;
  defaultOpen?: boolean;
}

/**
 * Inline instance card for AWS Bedrock. Renders Bedrock-specific fields
 * (auth_mode segmented, ak/sk/arn, region, model name, max tokens,
 * model_type) directly instead of going through the generic DynamicForm
 * path. All fields are editable from the start (no name-first lock);
 * the parent page's top Save button drives persistence through the
 * imperative ref API.
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
  const [selectedModelInfo, setSelectedModelInfo] = useState<IModelInfo[]>([]);
  const [catalogRevision, setCatalogRevision] = useState(0);

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
    }
  }, [providerName, isDraft]);

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
          bedrock_endpoint_type: z
            .enum(['runtime', 'mantle_openai', 'mantle_anthropic'])
            .default('runtime'),
          bedrock_endpoint_url: z.string().optional(),
          bedrock_discovery_endpoint_url: z.string().optional(),
          bedrock_region: z
            .string()
            .min(1, { message: tSetting('bedrockRegionMessage') }),
          llm_name: z.string(),
          max_tokens: z
            .number({
              required_error: tSetting('maxTokensMessage'),
              invalid_type_error: tSetting('maxTokensInvalidMessage'),
            })
            .nonnegative({ message: tSetting('maxTokensMinMessage') }),
          model_type: z.array(z.enum(['chat', 'embedding'])),
        })
        .superRefine((data, ctx) => {
          if (data.auth_mode !== 'bedrock_api_key') {
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
          }
          if (data.auth_mode === 'access_key_secret') {
            if (!data.bedrock_ak || !data.bedrock_ak.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockAKMessage'),
                path: ['bedrock_ak'],
              });
            }
            if (!data.bedrock_sk || !data.bedrock_sk.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockSKMessage'),
                path: ['bedrock_sk'],
              });
            }
          }
          if (data.auth_mode === 'iam_role') {
            if (!data.aws_role_arn || !data.aws_role_arn.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('awsRoleArnMessage'),
                path: ['aws_role_arn'],
              });
            }
          }
          if (data.auth_mode === 'bedrock_api_key') {
            if (!data.bedrock_api_key?.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockAPIKeyMessage'),
                path: ['bedrock_api_key'],
              });
            }
            if (
              data.bedrock_endpoint_type !== 'runtime' &&
              !data.bedrock_endpoint_url?.trim()
            ) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockEndpointURLMessage'),
                path: ['bedrock_endpoint_url'],
              });
            }
          }
        }),
    [tSetting],
  );

  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instance.id,
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
    const apiKey = parseApiKeyAsObject(merged.api_key) ?? {};
    return {
      auth_mode: (apiKey.auth_mode as AuthMode) ?? 'access_key_secret',
      bedrock_ak: apiKey.bedrock_ak ?? '',
      bedrock_sk: apiKey.bedrock_sk ?? '',
      aws_role_arn: apiKey.aws_role_arn ?? '',
      bedrock_api_key: apiKey.bedrock_api_key ?? '',
      bedrock_endpoint_type:
        (apiKey.bedrock_endpoint_type as BedrockEndpointType) ?? 'runtime',
      bedrock_endpoint_url: apiKey.bedrock_endpoint_url ?? '',
      bedrock_discovery_endpoint_url:
        apiKey.bedrock_discovery_endpoint_url ?? '',
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
  const [catalogCredentialsDirty, setCatalogCredentialsDirty] = useState(false);
  const catalogCredentialScopeRef = useRef(
    getBedrockCatalogCredentialScope(initialValues),
  );
  const persistedCatalogCredentialScopeRef = useRef(
    getBedrockCatalogCredentialScope(initialValues),
  );
  const previousInitialValuesRef = useRef(initialValues);
  const pendingCatalogResetRef = useRef<{
    scope: string;
    invalidateCatalog: boolean;
  } | null>(null);

  useEffect(() => {
    if (
      !shouldResetBedrockForm(previousInitialValuesRef.current, initialValues)
    )
      return;
    const nextScope = getBedrockCatalogCredentialScope(initialValues);
    pendingCatalogResetRef.current = {
      scope: nextScope,
      invalidateCatalog:
        persistedCatalogCredentialScopeRef.current !== nextScope,
    };
    previousInitialValuesRef.current = initialValues;
    persistedCatalogCredentialScopeRef.current = nextScope;
    setCatalogCredentialsDirty(false);
    form.reset(initialValues);
    // oxlint-disable-next-line react/exhaustive-deps
  }, [initialValues]);

  const authMode = useWatch({ control: form.control, name: 'auth_mode' });
  const endpointType = useWatch({
    control: form.control,
    name: 'bedrock_endpoint_type',
  });
  const bedrockAPIKey = useWatch({
    control: form.control,
    name: 'bedrock_api_key',
  });
  const bedrockRegion = useWatch({
    control: form.control,
    name: 'bedrock_region',
  });
  const endpointURL = useWatch({
    control: form.control,
    name: 'bedrock_endpoint_url',
  });
  const discoveryEndpointURL = useWatch({
    control: form.control,
    name: 'bedrock_discovery_endpoint_url',
  });
  const bedrockAccessKey = useWatch({
    control: form.control,
    name: 'bedrock_ak',
  });
  const bedrockSecretKey = useWatch({
    control: form.control,
    name: 'bedrock_sk',
  });
  const awsRoleArn = useWatch({ control: form.control, name: 'aws_role_arn' });

  const catalogCredentialScope = getBedrockCatalogCredentialScope({
    auth_mode: authMode,
    bedrock_endpoint_type: endpointType,
    bedrock_api_key: bedrockAPIKey,
    bedrock_region: bedrockRegion,
    bedrock_endpoint_url: endpointURL,
    bedrock_discovery_endpoint_url: discoveryEndpointURL,
    bedrock_ak: bedrockAccessKey,
    bedrock_sk: bedrockSecretKey,
    aws_role_arn: awsRoleArn,
  });

  useEffect(() => {
    const pendingReset = pendingCatalogResetRef.current;
    if (pendingReset) {
      if (pendingReset.scope !== catalogCredentialScope) return;
      pendingCatalogResetRef.current = null;
      catalogCredentialScopeRef.current = catalogCredentialScope;
      if (pendingReset.invalidateCatalog) {
        setSelectedModelInfo([]);
        setCatalogRevision((revision) => revision + 1);
      }
      return;
    }
    if (catalogCredentialScopeRef.current === catalogCredentialScope) return;
    catalogCredentialScopeRef.current = catalogCredentialScope;
    setSelectedModelInfo([]);
    setCatalogCredentialsDirty(
      !isDraft &&
        catalogCredentialScope !== persistedCatalogCredentialScopeRef.current,
    );
    setCatalogRevision((revision) => revision + 1);
  }, [catalogCredentialScope, isDraft]);

  const regionOptions = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: tSetting(x) })),
    [tSetting],
  );

  // Build a Bedrock-shaped payload for both submit and verify flows.
  const buildPayload = useCallback(
    (values: BedrockFormValues, instanceName: string) => {
      const cleaned: Record<string, any> = { ...values };
      const fieldsByMode: Record<AuthMode, string[]> = {
        access_key_secret: ['bedrock_ak', 'bedrock_sk'],
        iam_role: ['aws_role_arn'],
        assume_role: [],
        bedrock_api_key: [
          'bedrock_api_key',
          'bedrock_endpoint_type',
          'bedrock_endpoint_url',
          'bedrock_discovery_endpoint_url',
        ],
      };
      (Object.keys(fieldsByMode) as AuthMode[]).forEach((mode) => {
        if (mode !== values.auth_mode) {
          fieldsByMode[mode].forEach((f) => {
            delete cleaned[f];
          });
        }
      });
      if (
        values.auth_mode === 'bedrock_api_key' &&
        values.bedrock_endpoint_type !== 'runtime'
      ) {
        delete cleaned.bedrock_discovery_endpoint_url;
      }

      const flat = {
        ...cleaned,
        instance_name: instanceName,
        llm_factory: providerName,
        base_url: '',
        region: values.bedrock_region,
        max_tokens: values.max_tokens,
        model_type: values.model_type,
      };
      const { instancePayload, modelPayload } = splitProviderPayload(flat);
      return {
        ...instancePayload,
        max_tokens: modelPayload.max_tokens,
        model_info:
          values.auth_mode === 'bedrock_api_key'
            ? selectedModelInfo
            : [modelPayload],
      } as IAddProviderInstanceRequestBody;
    },
    [providerName, selectedModelInfo],
  );

  const { verifyProviderConnection } = useVerifyProviderConnection();
  const getModelsSectionValues = useCallback(() => {
    const values = form.getValues();
    const payload = buildPayload(
      values,
      draftName.trim() || instance.instance_name,
    );
    return {
      ...values,
      ...getBedrockModelListRequest(values),
      base_url: payload.base_url ?? '',
    };
  }, [form, buildPayload, draftName, instance.instance_name]);

  const transformModelVerify = useCallback(
    (values: Record<string, any>) => {
      const formValues = { ...values };
      delete formValues.api_key;
      delete formValues.extensions;
      const payload = buildPayload(
        formValues as BedrockFormValues,
        draftName.trim() || instance.instance_name,
      );
      return {
        apiKey: JSON.stringify(payload.api_key ?? ''),
        baseUrl: payload.base_url,
        region: payload.region,
      };
    },
    [buildPayload, draftName, instance.instance_name],
  );

  const handleVerify = useCallback(
    async (params: any) => {
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
      );
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: JSON.stringify(payload.api_key),
        base_url: payload.base_url,
        region: payload.region,
        model_info: payload.model_info,
        ...params,
      });
      return {
        isValid: ret.code === 0,
        logs: ret.message,
      } as VerifyResult;
    },
    [
      form,
      providerName,
      buildPayload,
      draftName,
      instance.instance_name,
      verifyProviderConnection,
      tSetting,
    ],
  );

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
    const baseline = buildPayload(baselineValues, instance.instance_name);
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
    instance.instance_name,
    instance.id,
    instanceDetails,
    providerName,
  ]);

  const getSavePayload = useCallback(() => {
    const trimmed = draftNameRef.current.trim();
    if (isDraft) {
      if (!trimmed) return null;
      const values = form.getValues();
      const payload = buildPayload(values, trimmed);
      return {
        payload,
        instanceName: trimmed,
        isDraft: true,
        // Bedrock drafts use the add endpoint (no id).
        apiKind: 'add' as const,
      };
    }
    // Collapsed saved cards never fetch their details, so their form still
    // contains incomplete defaults and must not participate in a batch save.
    // Once details are loaded, compare the actual payload with the baseline
    // instead of relying on formState.isDirty: React Hook Form only updates
    // that proxy field after a render-time subscription, while this value is
    // read exclusively through the imperative save ref.
    if (!instanceDetails) return null;
    const values = form.getValues();
    const payload = buildPayload(values, instance.instance_name);
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
    instance.instance_name,
    instance.id,
    instanceDetails,
    providerName,
  ]);

  const markSaved = useCallback(() => {
    const result = getSavePayload();
    if (result) {
      baselinePayloadRef.current = JSON.stringify(result.payload);
    }
    persistedCatalogCredentialScopeRef.current =
      catalogCredentialScopeRef.current;
    setCatalogCredentialsDirty(false);
    setCatalogRevision((revision) => revision + 1);
  }, [getSavePayload]);

  const modelsSectionInstanceName = instance.instance_name || '__draft__';

  useImperativeHandle(
    ref,
    () => ({
      validate: async () => {
        if (isDraft && !draftNameRef.current.trim()) return false;
        const isValid = await form.trigger();
        if (
          form.getValues('auth_mode') === 'bedrock_api_key' &&
          selectedModelInfo.length === 0
        ) {
          message.error(tSetting('selectModelBeforeVerify'));
          return false;
        }
        return !!isValid;
      },
      getSavePayload,
      markSaved,
    }),
    [
      isDraft,
      form,
      getSavePayload,
      markSaved,
      selectedModelInfo.length,
      tSetting,
    ],
  );

  const createEndpointTypeChangeHandler = useCallback(
    (onChange: (value: string) => void) =>
      function handleEndpointTypeChange(value: string) {
        if (value !== 'runtime') {
          form.setValue('bedrock_discovery_endpoint_url', '');
        }
        onChange(value);
      },
    [form],
  );

  const createMaxTokensChangeHandler = useCallback(
    (onChange: (value: number) => void) =>
      function handleMaxTokensChange(
        event: React.ChangeEvent<HTMLInputElement>,
      ) {
        onChange(Number(event.target.value));
      },
    [],
  );

  // ──────────────── Field group rendered in both modes ────────────────
  const renderFields = () => (
    <Form {...form}>
      <form className="space-y-6" onSubmit={(e) => e.preventDefault()}>
        {authMode !== 'bedrock_api_key' && (
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
                  if (value !== 'access_key_secret') {
                    form.setValue('bedrock_ak', '');
                    form.setValue('bedrock_sk', '');
                  }
                  if (value !== 'iam_role') {
                    form.setValue('aws_role_arn', '');
                  }
                  if (value !== 'bedrock_api_key') {
                    form.setValue('bedrock_api_key', '');
                    form.setValue('bedrock_endpoint_type', 'runtime');
                    form.setValue('bedrock_endpoint_url', '');
                    form.setValue('bedrock_discovery_endpoint_url', '');
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
                    label: tSetting('awsAuthModeBedrockAPIKey'),
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
              required
            >
              <Input placeholder={tSetting('bedrockAKMessage')} />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="bedrock_sk"
              label={tSetting('awsSecretAccessKey')}
              required
            >
              <Input placeholder={tSetting('bedrockSKMessage')} />
            </RAGFlowFormItem>
          </>
        )}

        {authMode === 'iam_role' && (
          <RAGFlowFormItem
            name="aws_role_arn"
            label={tSetting('awsRoleArn')}
            required
          >
            <Input placeholder={tSetting('awsRoleArnMessage')} />
          </RAGFlowFormItem>
        )}

        {authMode === 'assume_role' && (
          <div className="text-sm text-text-secondary">
            {tSetting('awsAssumeRoleTip')}
          </div>
        )}

        {authMode === 'bedrock_api_key' && (
          <>
            <RAGFlowFormItem
              name="bedrock_api_key"
              label={tSetting('bedrockAPIKey')}
              required
            >
              <Input
                type="password"
                placeholder={tSetting('bedrockAPIKeyMessage')}
              />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="bedrock_endpoint_type"
              label={tSetting('bedrockEndpointType')}
              required
            >
              {(field) => (
                <SelectWithSearch
                  value={field.value}
                  onChange={createEndpointTypeChangeHandler(field.onChange)}
                  options={[
                    {
                      value: 'runtime',
                      label: tSetting('bedrockEndpointRuntime'),
                    },
                    {
                      value: 'mantle_openai',
                      label: tSetting('bedrockEndpointMantleOpenAI'),
                    },
                    {
                      value: 'mantle_anthropic',
                      label: tSetting('bedrockEndpointMantleAnthropic'),
                    },
                  ]}
                />
              )}
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="bedrock_endpoint_url"
              label={tSetting('bedrockEndpointURL')}
              required={endpointType !== 'runtime'}
            >
              <Input placeholder={tSetting('bedrockEndpointURLMessage')} />
            </RAGFlowFormItem>
            {endpointType === 'runtime' && (
              <RAGFlowFormItem
                name="bedrock_discovery_endpoint_url"
                label={tSetting('bedrockDiscoveryEndpointURL')}
              >
                <Input
                  placeholder={tSetting('bedrockDiscoveryEndpointURLMessage')}
                />
              </RAGFlowFormItem>
            )}
          </>
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

        {authMode !== 'bedrock_api_key' && (
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
                onChange={createMaxTokensChangeHandler(field.onChange)}
              />
            )}
          </RAGFlowFormItem>
        )}
      </form>

      {/* VerifyButton lives inside <Form> (FormProvider) so its
          internal useFormContext() resolves the form instance.
          Rendered outside <form> so it never triggers submission. */}
      <div className="pt-3">
        <VerifyButton onVerify={handleVerify} isAbsolute={false} />
      </div>
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
              key={catalogRevision}
              providerName={providerName}
              instanceName={modelsSectionInstanceName}
              instance={instance}
              hideActions={false}
              hideIfEmpty={false}
              getFormValues={getModelsSectionValues}
              verifyTransform={transformModelVerify}
              onInstanceModelsChange={setSelectedModelInfo}
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

              <div className="pt-3">
                <ModelsSection
                  key={catalogRevision}
                  providerName={providerName}
                  instanceName={modelsSectionInstanceName}
                  instance={instance}
                  hideActions={false}
                  deferModelMutations={!isDraft && catalogCredentialsDirty}
                  hideIfEmpty={false}
                  getFormValues={getModelsSectionValues}
                  verifyTransform={transformModelVerify}
                  onInstanceModelsChange={setSelectedModelInfo}
                />
              </div>
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
