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

import { DynamicFormRef, FormFieldType } from '@/components/dynamic-form';
import {
  useDeleteProviderInstance,
  useFetchAvailableProviders,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { useTranslate } from '@/hooks/common-hooks';
import { IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo } from '@/interfaces/request/llm';
import { RefObject, useCallback, useEffect, useMemo, useRef } from 'react';
import { getProviderConfig } from '../provider-schema/field-config';
import { useProviderFields } from '../provider-schema/hooks';
import { SelectOption } from '../provider-schema/types';
import type { ControllerRenderProps } from 'react-hook-form';
import {
  API_KEY_NESTED_FIELDS,
  ApiKeyNestedField,
  InstanceSavePayload,
} from './interface';
import {
  SAVED_CREDENTIAL_MASK,
  SavedCredentialInput,
} from './saved-credential-input';
import {
  SavedProviderConfigFieldSet,
  hasUnsavedProviderCredential,
  pickSavedProviderConfigOverrides,
} from './saved-provider-config';

export { SAVED_CREDENTIAL_MASK } from './saved-credential-input';

// ---------------------------------------------------------------------------
// Pure helpers - api_key shape normalization
// ---------------------------------------------------------------------------

/**
 * Build the `api_key` payload value from the flat form values. If any of
 * the nested credential fields (see `API_KEY_NESTED_FIELDS`) carry a
 * value, returns `{ api_key, ...nested }`; otherwise returns the plain
 * api_key string. Used by both the payload builder and its change
 * signature baseline so the two stay byte-identical.
 */
export function buildApiKeyValue(
  values: Record<string, any>,
  credentialStatus: Record<string, boolean> = {},
): string | Record<string, any> | undefined {
  const config: Record<string, any> = {};
  const apiKey = values.api_key;
  if (credentialStatus.api_key === true) {
    if (apiKey !== SAVED_CREDENTIAL_MASK && apiKey !== undefined) {
      config.api_key = apiKey === '' ? null : apiKey;
    }
  } else if (apiKey !== undefined && apiKey !== '') {
    config.api_key = apiKey;
  }

  for (const field of API_KEY_NESTED_FIELDS) {
    const v = values[field];
    if (credentialStatus[field] === true) {
      if (v !== SAVED_CREDENTIAL_MASK && v !== undefined) {
        config[field] = v === '' ? null : v;
      }
    } else if (v !== undefined && v !== '') {
      config[field] = v;
    }
  }

  const keys = Object.keys(config);
  if (keys.length === 0) {
    return apiKey === SAVED_CREDENTIAL_MASK ? undefined : apiKey;
  }
  if (keys.length === 1 && keys[0] === 'api_key' && config.api_key !== null) {
    return config.api_key;
  }
  return config;
}

/**
 * Apply edits to credentials already stored in a provider's structured
 * `api_key` value. Unchanged masks are omitted so the backend preserves those
 * fields; an emptied field is sent as null so the backend removes it.
 */
export function applySavedCredentialEdits(
  apiKey: unknown,
  values: Record<string, any>,
  credentialStatus: Record<string, boolean> = {},
): unknown {
  const changes: Record<string, any> = {};
  for (const [field, configured] of Object.entries(credentialStatus)) {
    if (!configured) continue;
    const value = values[field];
    if (value === undefined || value === SAVED_CREDENTIAL_MASK) continue;
    changes[field] = value === '' || value === null ? null : value;
  }
  if (Object.keys(changes).length === 0) {
    if (
      typeof apiKey === 'string' &&
      credentialStatus.api_key === true &&
      values.api_key === SAVED_CREDENTIAL_MASK
    ) {
      return undefined;
    }
    if (apiKey && typeof apiKey === 'object' && !Array.isArray(apiKey)) {
      const config = { ...(apiKey as Record<string, any>) };
      for (const [field, configured] of Object.entries(credentialStatus)) {
        if (configured && config[field] === SAVED_CREDENTIAL_MASK) {
          delete config[field];
        }
      }
      return config;
    }
    if (typeof apiKey === 'string' && apiKey.trim() === '') {
      return undefined;
    }
    return apiKey;
  }

  let config: Record<string, any> = {};
  if (apiKey && typeof apiKey === 'object' && !Array.isArray(apiKey)) {
    config = { ...(apiKey as Record<string, any>) };
  } else if (typeof apiKey === 'string' && apiKey !== '') {
    try {
      const parsed = JSON.parse(apiKey);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        config = parsed;
      } else {
        config.api_key = apiKey;
      }
    } catch {
      config.api_key = apiKey;
    }
  }
  Object.assign(config, changes);
  for (const [field, configured] of Object.entries(credentialStatus)) {
    if (configured && config[field] === SAVED_CREDENTIAL_MASK) {
      delete config[field];
    }
  }
  if (
    Object.keys(config).length === 1 &&
    Object.prototype.hasOwnProperty.call(config, 'api_key') &&
    config.api_key !== null
  ) {
    return config.api_key;
  }
  return config;
}

/**
 * Parse provider-specific fields from a safe configuration object. The
 * detail API never returns stored credential values.
 */
export function unwrapApiKey(raw: unknown): {
  apiKey: string;
  nested: Record<string, any>;
} {
  let obj: any = raw;
  if (typeof raw === 'string') {
    const trimmed = raw.trim();
    if (trimmed.startsWith('{')) {
      try {
        obj = JSON.parse(trimmed);
      } catch {
        obj = raw;
      }
    }
  }
  if (obj && typeof obj === 'object') {
    const nested: Record<string, any> = {};
    for (const field of API_KEY_NESTED_FIELDS) {
      const v = obj[field];
      if (v !== undefined && v !== '') nested[field] = v;
    }
    return { apiKey: obj.api_key ?? '', nested };
  }
  return { apiKey: typeof raw === 'string' ? raw : '', nested: {} };
}

/** Pick the value associated with the `default` region, if present. */
function pickDefaultUrl(
  options?: Array<{ value: string; regionKey?: string }>,
): string | undefined {
  // Guard against non-array data (e.g. a transient cache/HMR value).
  return Array.isArray(options)
    ? options.find((o) => o.regionKey === 'default')?.value
    : undefined;
}

// ---------------------------------------------------------------------------
// useProviderBaseUrlOptions - fetch provider catalog and build URL options
// ---------------------------------------------------------------------------

/**
 * Fetch the catalog of available providers and derive the
 * `base_url` dropdown options for the current provider.
 * Used to pre-fill the URL field with the provider's default URL when
 * creating a new instance.
 *
 * Also exposes `url_hint`: a display-only example endpoint that becomes
 * the endpoint input's placeholder. Providers that ship no default URL
 * (most self-hosted ones) still get a useful hint.
 */
export function useProviderBaseUrlOptions(providerName: string) {
  const { data: availableProviders } = useFetchAvailableProviders();

  const currentProvider = useMemo(
    () =>
      providerName
        ? availableProviders.find((p) => p.name === providerName)
        : undefined,
    [availableProviders, providerName],
  );

  const baseUrlOptions = useMemo(() => {
    const urlObj = currentProvider?.url as
      | Record<string, string | undefined>
      | undefined;
    if (!urlObj) return undefined;
    const options = Object.keys(urlObj)
      .filter((k) => !!urlObj[k])
      .map((k) => {
        const v = urlObj[k] as string;
        return {
          value: v,
          regionKey: k,
          label: (
            <div className="flex justify-between items-center gap-2">
              <span className="truncate">{v}</span>
              <span className="text-xs text-text-secondary bg-bg-card px-2 py-0.5 rounded-sm shrink-0">
                {k}
              </span>
            </div>
          ),
        };
      });
    return options.length > 0 ? options : undefined;
  }, [currentProvider]);

  const urlHint = useMemo(() => currentProvider?.url_hint, [currentProvider]);

  return { baseUrlOptions, availableProviders, urlHint };
}

// ---------------------------------------------------------------------------
// useProviderInitialValues - form initial values for draft vs saved
// ---------------------------------------------------------------------------

/**
 * Build the form's initial values from the persisted instance, the
 * lazy-loaded `showProviderInstance` details, and the provider's default
 * base URL.
 *
 * - Draft: empty form, with `base_url` pre-filled from the
 *   provider's `default` URL when available.
 * - Saved: prefer `instanceDetails` (safe config status plus base_url);
 *   when `echoTransform` is supplied, use it to restore provider-specific
 *   fields from the safe configuration. Otherwise fall back to the generic
 *   `unwrapApiKey` path, which lifts settings registered in
 *   `API_KEY_NESTED_FIELDS`.
 */
export function useProviderInitialValues(
  providerName: string,
  instance: IProviderInstance,
  instanceDetails: IProviderInstance | undefined,
  isDraft: boolean,
  baseUrlOptions: SelectOption[] | undefined,
  echoTransform?: (instance: Record<string, any>) => Record<string, any>,
) {
  return useMemo(() => {
    const defaultBaseUrl = pickDefaultUrl(baseUrlOptions);

    if (isDraft) {
      const values: Record<string, any> = { instance_name: '' };
      if (defaultBaseUrl) {
        // Seed the URL field by the name the provider's form actually
        // uses. Most providers call it `base_url`, but some (e.g.
        // PaddleOCR) use a provider-specific field name.
        const urlFieldName =
          getProviderConfig(providerName).fields.find(
            (f) => f.type === 'inputSelect',
          )?.name ?? 'base_url';
        values[urlFieldName] = defaultBaseUrl;
      }
      return values;
    }

    const merged: IProviderInstance = {
      ...instance,
      ...(instanceDetails ?? ({} as IProviderInstance)),
    };
    const values: Record<string, any> = {
      instance_name: merged.instance_name,
      api_key: '',
      api_key_configured: merged.api_key_configured ?? false,
      api_key_config: merged.api_key_config ?? {},
      credential_status: merged.credential_status ?? {},
      credential_masks: merged.credential_masks ?? {},
    };
    const safeConfig = merged.api_key_config ?? {};
    const { nested } = unwrapApiKey(safeConfig);
    Object.assign(values, nested);
    if (echoTransform) {
      // Provider-specific echo: reverse the `submitTransform` mapping so
      // provider-specific fields are restored from the safe config or
      // nested `api_key` values.
      Object.assign(values, echoTransform({ ...merged, ...safeConfig }));
    }
    if (merged.base_url) {
      values.base_url = merged.base_url;
    } else if (defaultBaseUrl) {
      values.base_url = defaultBaseUrl;
    }
    // The /providers/<p>/instances/<i> endpoint also returns `region`
    // for providers where it applies; surface it so the form / region
    // submit logic can echo it back.
    if ((merged as any).region) values.region = (merged as any).region;
    // Restore only known non-secret settings from the safe config map.
    for (const field of API_KEY_NESTED_FIELDS) {
      if (values[field] === undefined && safeConfig[field] !== undefined) {
        values[field] = safeConfig[field];
      }
    }
    for (const [field, configured] of Object.entries(
      values.credential_status,
    )) {
      if (configured === true) values[field] = SAVED_CREDENTIAL_MASK;
    }
    return values;
  }, [
    providerName,
    instance,
    instanceDetails,
    isDraft,
    baseUrlOptions,
    echoTransform,
  ]);
}

// ---------------------------------------------------------------------------
// useLazyInstanceDetails - fetch full instance details when card opens
// ---------------------------------------------------------------------------

/**
 * The list endpoint (`useFetchProviderInstances`) does not return
 * sensitive/heavy fields like `api_key` or `base_url`. Pull the full
 * instance via `showProviderInstance` to get safe config status and URL
 * metadata when the user clicks an existing provider on the left. The hook is
 * `enabled: false` by default - we trigger it manually here so we
 * don't change behavior of other call sites.
 */
export function useLazyInstanceDetails(
  providerName: string,
  instanceId: string,
  isDraft: boolean,
  open: boolean,
) {
  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instanceId,
    );

  useEffect(() => {
    if (!isDraft && open && providerName && instanceId) {
      refetchInstanceDetails();
    }
  }, [isDraft, open, providerName, instanceId, refetchInstanceDetails]);

  return { instanceDetails, refetchInstanceDetails };
}

// ---------------------------------------------------------------------------
// useFormResetOnDetailsLoad - re-fill form when instanceDetails resolves
// ---------------------------------------------------------------------------

/**
 * React-Hook-Form only consumes `defaultValues` on first mount, so we
 * explicitly reset the form here to make the freshly-fetched values
 * visible. We use `keepDirtyValues` so the user's in-progress edits
 * (if any) are not clobbered by a background refetch.
 */
export function useFormResetOnDetailsLoad(
  formRef: RefObject<DynamicFormRef>,
  formDefaultValues: Record<string, any>,
  instanceDetails: IProviderInstance | undefined,
  isDraft: boolean,
) {
  useEffect(() => {
    if (isDraft) return;
    if (!instanceDetails) return;
    const form = (formRef.current as any)?.form;
    if (form?.reset) {
      form.reset(formDefaultValues, { keepDirtyValues: true });
    } else {
      formRef.current?.reset?.(formDefaultValues);
    }
  }, [isDraft, instanceDetails, formDefaultValues, formRef]);
}

// ---------------------------------------------------------------------------
// useVerifyProvider - wraps useVerifyProviderConnection for the card
// ---------------------------------------------------------------------------

/** Optional transform supplied by the provider config. When present it
 *  maps provider-specific form field names (e.g. OpenDataLoader's
 *  `opendataloader_apiserver`) onto the `{ apiKey, baseUrl, modelInfo }`
 *  shape the verify endpoint expects. When absent the generic mapping
 *  (`values.api_key` / `values.base_url`) is used. */
type VerifyTransform = (values: Record<string, any>) => {
  apiKey: string | object | Record<string, any>;
  baseUrl?: string;
  region?: string;
  modelInfo?: IModelInfo[];
};

/**
 * Adapter that reads the current form values and proxies them into
 * `verifyProviderConnection`, then shapes the response into the
 * `VerifyResult` consumed by {@link VerifyButton}.
 *
 * When `verifyTransform` is supplied (provider-specific field mapping,
 * e.g. OpenDataLoader's nested `opendataloader_apiserver` /
 * `opendataloader_api_key`), it is used to build the verify args;
 * otherwise the generic `values.api_key` / `values.base_url` mapping
 * is used.
 *
 * `modelInfoRef` carries the models selected in the List-models picker
 * (not registered as form fields). Without it, verify payloads for
 * local providers often omit `model_info` and the backend falls back
 * to an empty factory catalog.
 */
export function useVerifyProvider(
  providerName: string,
  formRef: RefObject<DynamicFormRef>,
  verifyTransform?: VerifyTransform,
  modelInfoRef?: { current: IModelInfo[] },
  instanceId?: string,
  savedInstance?: IProviderInstance,
) {
  const { verifyProviderConnection } = useVerifyProviderConnection();

  return useCallback(
    async (params: any) => {
      const values = { ...(formRef.current?.getValues?.() ?? {}), ...params };
      const selectedModels =
        modelInfoRef?.current?.length && modelInfoRef.current.length > 0
          ? modelInfoRef.current
          : undefined;
      if (selectedModels && !values.model_info) {
        values.model_info = selectedModels;
      }
      let verifyArgs: {
        api_key: string | object;
        base_url?: string;
        model_info?: IModelInfo[];
        region?: string;
      };
      const credentialStatus = savedInstance?.credential_status ?? {};
      const verifyValues = { ...values };
      for (const [field, configured] of Object.entries(credentialStatus)) {
        if (configured && verifyValues[field] === SAVED_CREDENTIAL_MASK) {
          verifyValues[field] = '';
        }
      }
      if (verifyTransform) {
        const transformed = verifyTransform(verifyValues);
        verifyArgs = {
          api_key: transformed.apiKey,
          base_url: transformed.baseUrl,
          model_info: transformed.modelInfo?.length
            ? transformed.modelInfo
            : (selectedModels ?? values.model_info),
          region: transformed.region,
        };
      } else {
        verifyArgs = {
          api_key: values.api_key ?? '',
          base_url: values.base_url,
          model_info: selectedModels ?? values.model_info,
        };
      }
      verifyArgs.api_key = applySavedCredentialEdits(
        verifyArgs.api_key,
        values,
        credentialStatus,
      ) as string | object;
      const credentialChanged = Object.keys(credentialStatus).some(
        (field) =>
          credentialStatus[field] === true &&
          !SavedProviderConfigFieldSet.has(field) &&
          values[field] !== undefined &&
          values[field] !== SAVED_CREDENTIAL_MASK,
      );
      const useSavedCredentials =
        Boolean(instanceId) &&
        !credentialChanged &&
        !hasUnsavedProviderCredential(verifyArgs.api_key);
      const canOverrideSavedCredentials =
        useSavedCredentials && savedInstance?.is_owner === true;
      const savedConfigOverrides = canOverrideSavedCredentials
        ? pickSavedProviderConfigOverrides(verifyArgs.api_key)
        : {};
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        ...(instanceId ? { instance_id: instanceId } : {}),
        ...(useSavedCredentials
          ? {
              use_saved_credentials: true,
              ...(Object.keys(savedConfigOverrides).length > 0
                ? { api_key_config: savedConfigOverrides }
                : {}),
              ...(canOverrideSavedCredentials
                ? { base_url: verifyArgs.base_url, region: verifyArgs.region }
                : {}),
            }
          : (verifyArgs as any)),
      });
      if (ret.code === 0) {
        return { isValid: true, logs: ret.message } as {
          isValid: boolean;
          logs: string;
        };
      }
      return { isValid: false, logs: ret.message } as {
        isValid: boolean;
        logs: string;
      };
    },
    [
      providerName,
      formRef,
      verifyProviderConnection,
      verifyTransform,
      modelInfoRef,
      instanceId,
      savedInstance,
    ],
  );
}

// ---------------------------------------------------------------------------
// useDeleteInstance - wires the card's delete button
// ---------------------------------------------------------------------------

/**
 * Delete handler: for saved instances calls `useDeleteProviderInstance`;
 * for drafts calls `onDelete` (which maps to onCancel in the parent).
 */
export function useDeleteInstance(
  providerName: string,
  instanceName: string,
  isDraft: boolean,
  onDelete?: () => void,
) {
  const { deleteProviderInstance } = useDeleteProviderInstance();
  return useCallback(async () => {
    if (isDraft) {
      onDelete?.();
    } else {
      await deleteProviderInstance({
        provider_name: providerName,
        instances: [instanceName],
      });
    }
  }, [isDraft, providerName, instanceName, deleteProviderInstance, onDelete]);
}

// ---------------------------------------------------------------------------
// useInstanceSaveState - payload builder + dirty tracking (no auto-save)
// ---------------------------------------------------------------------------

interface UseInstanceSaveStateArgs {
  formRef: RefObject<DynamicFormRef>;
  providerName: string;
  /** Persisted instance name - used for the dirty-tracking baseline. */
  instanceName: string;
  /**
   * The instance name currently shown in the UI (may differ from
   * `instanceName` when the user has double-clicked to rename a saved
   * card). Used in the save payload so a rename is persisted. Falls
   * back to `instanceName` when not provided (drafts use `draftName`).
   */
  editedInstanceName?: string;
  instanceId: string | undefined;
  isDraft: boolean;
  draftName: string;
  instanceDetails: IProviderInstance | undefined;
  initialValues: Record<string, any>;
  baselineValues?: Record<string, any>;
  modelInfoRef: { current: IModelInfo[] };
  /**
   * Optional provider-specific transform that maps form values to the
   * submit API body (e.g. OpenDataLoader's nested
   * `opendataloader_apiserver` / `opendataloader_api_key` fields). When
   * absent the generic `buildApiKeyValue` + `values.base_url` mapping is
   * used.
   */
  submitTransform?: (values: Record<string, any>) => Record<string, any>;
}

/**
 * Owns payload construction + dirty tracking for a single instance card.
 *
 * Replaces the old `useDraftAutoSave` + `useSavedAutoSave` pair. The
 * auto-save effects are gone - the parent page now drives save
 * explicitly through the imperative ref API. What remains is:
 *   - `buildPayload()`: assemble the API body from current form values.
 *   - `getSavePayload()`: return the body if dirty (or a draft with a
 *     name), else `null` so the parent can skip the redundant call.
 *   - `markSaved()`: re-baseline after a successful save.
 *   - `markModelsEdited()`: absorb the authoritative saved model snapshot
 *     into the baseline so the next top-save does not re-PUT model_info
 *     that the backend already persisted.
 *
 * The dirty check compares a JSON signature of the current payload to
 * the baseline signature, mirroring the old `lastSavedPayloadRef`
 * approach. `model_info` is folded into the signature so editing a
 * nested credential field (e.g. MiniMax `group_id`, which is nested
 * inside `api_key` by `buildApiKeyValue`) still counts as dirty.
 */
export function useInstanceSaveState({
  formRef,
  providerName,
  instanceName,
  editedInstanceName,
  instanceId,
  isDraft,
  draftName,
  instanceDetails,
  initialValues,
  baselineValues,
  modelInfoRef,
  submitTransform,
}: UseInstanceSaveStateArgs) {
  const baselinePayloadRef = useRef<string>('');
  const draftNameRef = useRef(draftName);
  useEffect(() => {
    draftNameRef.current = draftName;
  });
  // Keep the latest edited name in a ref so `buildPayload` reads the
  // current value without being recreated on every keystroke.
  const editedNameRef = useRef(editedInstanceName ?? instanceName);
  useEffect(() => {
    editedNameRef.current = editedInstanceName ?? instanceName;
  });

  // Build the API payload from the current form values. For drafts the
  // body targets `addProviderInstance` (no `id`, `llm_factory` at the
  // top level); for saved cards it targets `updateProviderInstance`
  // (`provider_name`, `id`, `verify: false`). `api_key` is normalised
  // via `buildApiKeyValue` so nested credential fields (group_id /
  // api_version / provider_order) are bundled inside `api_key` as the
  // backend expects.
  const buildPayload = useCallback(
    (
      valuesOverride?: Record<string, any>,
      modelInfoOverride?: IModelInfo[],
      instanceNameOverride?: string,
      instanceIdOverride?: string,
    ): Record<string, any> | null => {
      const values =
        valuesOverride ??
        ((formRef.current?.getValues?.() ?? {}) as Record<string, any>);
      const modelInfo =
        modelInfoOverride ??
        (modelInfoRef.current.length > 0 ? modelInfoRef.current : []);

      // Provider-specific field mapping (e.g. OpenDataLoader's nested
      // `opendataloader_apiserver` / `opendataloader_api_key`). The
      // transform produces the canonical submit body shape
      // (`instance_name`, `llm_factory`, `api_key`, `base_url`,
      // `model_info`); we then layer on the card's own state (typed /
      // edited name, model_info ref, update-only fields for saved cards).
      if (submitTransform) {
        const credentialStatus = initialValues.credential_status ?? {};
        const transformValues = { ...values };
        for (const [field, configured] of Object.entries(credentialStatus)) {
          if (configured && transformValues[field] === SAVED_CREDENTIAL_MASK) {
            transformValues[field] = '';
          }
        }
        const transformed = submitTransform({
          ...transformValues,
          model_info: modelInfo,
        }) as Record<string, any>;
        const updatedAPIKey = applySavedCredentialEdits(
          transformed.api_key,
          values,
          credentialStatus,
        );
        if (updatedAPIKey === undefined) {
          delete transformed.api_key;
        } else {
          transformed.api_key = updatedAPIKey;
        }
        for (const [field, configured] of Object.entries(credentialStatus)) {
          // `api_key` is the canonical credential property in the update body.
          // Other provider-specific credential fields must not leak at the top
          // level, but deleting this one also removes the transformed value.
          if (configured && field !== 'api_key') delete transformed[field];
        }
        if (isDraft) {
          const trimmed = draftNameRef.current.trim();
          if (!trimmed) return null;
          return {
            ...transformed,
            llm_factory: providerName,
            instance_name: trimmed,
            model_info: modelInfo,
          };
        }
        const resolvedId =
          instanceIdOverride || instanceDetails?.id || instanceId;
        return {
          ...transformed,
          provider_name: providerName,
          instance_name: instanceNameOverride ?? editedNameRef.current,
          id: resolvedId,
          base_url: transformed.base_url ?? '',
          region: values.region || 'default',
          model_info: modelInfo,
          verify: false,
        };
      }

      if (isDraft) {
        const trimmed = draftNameRef.current.trim();
        if (!trimmed) return null;
        const payload: Record<string, any> = {
          llm_factory: providerName,
          instance_name: instanceNameOverride ?? trimmed,
          api_key:
            buildApiKeyValue(values, initialValues.credential_status) ?? '',
          base_url: values.base_url,
        };
        if (modelInfo.length > 0) {
          payload.model_info = modelInfo;
        }
        return payload;
      }

      const resolvedId =
        instanceIdOverride || instanceDetails?.id || instanceId;
      const apiKeyValue = buildApiKeyValue(
        values,
        initialValues.credential_status,
      );
      const payload: Record<string, any> = {
        provider_name: providerName,
        // Use the edited name (may differ from the persisted `instanceName`
        // when the user has renamed the instance via double-click).
        instance_name: instanceNameOverride ?? editedNameRef.current,
        id: resolvedId,
        base_url: values.base_url,
        region: values.region || 'default',
        model_info: modelInfo,
        verify: false,
      };
      if (
        apiKeyValue !== undefined &&
        !(typeof apiKeyValue === 'string' && apiKeyValue.trim() === '')
      ) {
        payload.api_key = apiKeyValue;
      }
      return payload;
    },
    [
      isDraft,
      providerName,
      instanceId,
      instanceDetails?.id,
      formRef,
      modelInfoRef,
      submitTransform,
      initialValues.credential_status,
    ],
  );

  // Seed the "last saved" baseline once instance details (or the
  // draft's empty state) are available, so the first `getSavePayload()`
  // after mount doesn't flag a phantom dirty. For drafts the baseline
  // stays empty - a draft is always considered dirty once it has a
  // name, so the baseline is only consulted for saved cards.
  useEffect(() => {
    if (isDraft) {
      baselinePayloadRef.current = '';
      return;
    }
    const resolvedId = instanceDetails?.id || instanceId;
    if (!resolvedId) return;
    // Build the baseline through the same provider transform and payload
    // normalization as a save. Hand-assembling it misses provider-specific
    // fields such as llm_factory and can make an untouched card look dirty.
    // Use the form defaults from the loaded instance, including provider
    // defaults, but not the live form values: a refetch must not absorb edits
    // preserved by `keepDirtyValues` into the saved baseline.
    const savedValues = baselineValues ?? initialValues;
    const baseline = buildPayload(savedValues, [], instanceName, resolvedId);
    baselinePayloadRef.current = JSON.stringify(baseline ?? {});
  }, [
    isDraft,
    instanceName,
    instanceId,
    instanceDetails?.id,
    initialValues,
    baselineValues,
    formRef,
    buildPayload,
  ]);

  // `getSavePayload()` is the imperative entry point the parent calls
  // when the user clicks the top Save button. For drafts it always
  // returns a payload (provided the name is non-empty); for saved
  // cards it returns `null` when the current payload matches the
  // last-synced baseline, so the parent skips the no-op PUT.
  //
  // Two guards skip cards that should not participate in the batch:
  //
  // 1. `!instanceDetails` - collapsed cards never lazy-fetch their
  //    details (see `useLazyInstanceDetails`), so their form is empty
  //    (api_key defaults to ''). Including them would (a) fail
  //    validation on the required api_key field and abort the entire
  //    batch, and (b) potentially send an empty api_key to the
  //    backend. A user cannot have edited a card they never opened,
  //    so skipping is always safe. Cards that were opened and then
  //    collapsed still retain their cached `instanceDetails`, so
  //    in-flight edits are not lost.
  //
  // 2. Signature comparison - the reliable dirty check. We
  //    intentionally do NOT short-circuit on `form.formState.isDirty`
  //    here. RHF only updates `_formState.isDirty` on field changes
  //    when a component subscribes to `isDirty` in a render (via the
  //    formState Proxy). This card reads `isDirty` only through the
  //    imperative ref (in the parent's click handler), so the
  //    subscription is never established and `_formState.isDirty`
  //    stays stale at its post-`reset` value (`false`) - causing
  //    api_key / base_url edits to be silently skipped. The signature
  //    comparison compares the actual payload (api_key, base_url,
  //    model_info, instance_name, etc.) against the baseline captured
  //    at last sync, so it catches every real edit.
  const getSavePayload = useCallback((): InstanceSavePayload | null => {
    if (!isDraft && !instanceDetails) return null;
    const payload = buildPayload();
    if (!payload) return null;
    if (!isDraft) {
      const sig = JSON.stringify(payload);
      if (sig === baselinePayloadRef.current) return null;
    }
    return {
      payload,
      instanceName: isDraft
        ? draftNameRef.current.trim()
        : editedNameRef.current,
      isDraft,
      // Generic drafts go through `addProviderInstance`; generic saved
      // cards go through `updateProviderInstance` (their payload matches
      // `IUpdateProviderInstanceRequestBody`).
      apiKind: isDraft ? 'add' : 'update',
    };
  }, [buildPayload, isDraft, instanceDetails]);

  // After a successful save the parent calls `markSaved()` so the
  // baseline catches up to the just-persisted values. Without this,
  // the next `getSavePayload()` would re-fire the same PUT.
  const markSaved = useCallback(() => {
    const payload = buildPayload();
    if (payload) {
      baselinePayloadRef.current = JSON.stringify(payload);
    }
  }, [buildPayload]);

  // Absorb the latest backend-fetched model list into the baseline. By
  // parsing the previously-saved baseline and overwriting ONLY model_info,
  // the signature check in `getSavePayload` short-circuits for an unchanged
  // card while any in-flight api_key / base_url / region edits still differ
  // from the baseline and remain eligible for saving.
  //
  // Skipped for drafts (the baseline is empty there) and until the
  // baseline has been seeded.
  const markModelsEdited = useCallback(() => {
    if (isDraft) return;
    const prev = baselinePayloadRef.current;
    if (!prev) return;
    try {
      const parsed = JSON.parse(prev) as Record<string, any>;
      parsed.model_info =
        modelInfoRef.current.length > 0 ? modelInfoRef.current : [];
      parsed.verify = false;
      baselinePayloadRef.current = JSON.stringify(parsed);
    } catch {
      // ignore parse errors - baseline will be re-seeded on next details load
    }
  }, [isDraft, modelInfoRef]);

  return {
    buildPayload,
    getSavePayload,
    markSaved,
    markModelsEdited,
  };
}

// ---------------------------------------------------------------------------
// useFormFields - wraps useProviderFields and strips instance_name
// ---------------------------------------------------------------------------

/**
 * Wraps `useProviderFields` and removes the `instance_name` field from
 * both the field list and the default values - the card header owns
 * the instance name (editable on hover), so we keep a single source of
 * truth and avoid showing it twice in the form.
 */
export function useFormFields(
  providerName: string,
  isDraft: boolean,
  initialValues: Record<string, any>,
  baseUrlOptions: SelectOption[] | undefined,
  hideWhenInstanceExists: (values: any) => boolean,
  urlHint?: string,
) {
  const { t: tSetting } = useTranslate('setting');
  const { fields, defaultValues } = useProviderFields({
    llmFactory: providerName,
    // Always seed initial values (drafts need the default base_url;
    // saved cards need safe persisted settings and the base_url).
    editMode: true,
    // Never disable fields - the old name-first lock is gone. Drafts
    // are fully editable from the start; saved cards are edited via the
    // top Save button.
    viewMode: false,
    initialValues,
    baseUrlOptions,
    hideWhenInstanceExists,
    urlHint,
  });

  const formFields = useMemo(() => {
    const credentialStatus = initialValues.credential_status ?? {};
    const credentialMasks = initialValues.credential_masks ?? {};
    return fields
      .filter((field) => field.name !== 'instance_name')
      .map((field) => {
        if (isDraft || credentialStatus[field.name] !== true) return field;
        return {
          ...field,
          type: FormFieldType.Custom,
          required: false,
          placeholder: tSetting('credentialClearToRemove'),
          autoComplete: 'new-password',
          render: (fieldProps: ControllerRenderProps) => (
            <SavedCredentialInput
              {...fieldProps}
              maskedValue={credentialMasks[field.name]}
              placeholder={tSetting('credentialClearToRemove')}
              autoComplete="new-password"
            />
          ),
        };
      });
  }, [
    fields,
    initialValues.credential_masks,
    initialValues.credential_status,
    isDraft,
    tSetting,
  ]);

  const defaultValuesKey = JSON.stringify(defaultValues);
  const formDefaultValues = useMemo(() => {
    const { instance_name: _ignored, ...rest } = (defaultValues ??
      {}) as Record<string, any>;
    void _ignored;
    return rest;
    // oxlint-disable-next-line react/exhaustive-deps
  }, [defaultValuesKey]);

  return { formFields, formDefaultValues };
}

// Re-export ApiKeyNestedField for components that need it.
export type { ApiKeyNestedField };
