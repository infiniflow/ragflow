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

import { DynamicFormRef } from '@/components/dynamic-form';
import {
  useDeleteProviderInstance,
  useFetchAvailableProviders,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { IModelInfo } from '@/interfaces/request/llm';
import { RefObject, useCallback, useEffect, useMemo, useRef } from 'react';
import { useProviderFields } from '../provider-schema/hooks';
import { SelectOption } from '../provider-schema/types';
import {
  API_KEY_NESTED_FIELDS,
  ApiKeyNestedField,
  InstanceSavePayload,
} from './interface';

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
): string | Record<string, any> | undefined {
  const nested: Record<string, any> = {};
  for (const field of API_KEY_NESTED_FIELDS) {
    const v = values[field];
    if (v !== undefined && v !== '') nested[field] = v;
  }
  if (Object.keys(nested).length === 0) return values.api_key;
  return { api_key: values.api_key ?? '', ...nested };
}

/**
 * Inverse of `buildApiKeyValue`, used on the echo/prefill path. The
 * backend persists the credential bundle as a JSON string (see
 * `rag/llm/chat_model.py`, which does `json.loads(key)`), so
 * `showProviderInstance` may return `api_key` as:
 *   1. a raw JSON string  `'{"api_key":"sk-x","group_id":"123"}'`
 *   2. an already-parsed object `{ api_key, group_id }`
 *   3. a plain bare key string `'sk-x'`
 * Normalise all three into the bare key plus any nested credential
 * fields so the form pre-fills group_id / api_version / provider_order.
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
  return options?.find((o) => o.regionKey === 'default')?.value;
}

// ---------------------------------------------------------------------------
// useProviderBaseUrlOptions - fetch provider catalog and build URL options
// ---------------------------------------------------------------------------

/**
 * Fetch the catalog of available providers and derive the
 * `base_url` / `api_base` dropdown options for the current provider.
 * Used to pre-fill the URL field with the provider's default URL when
 * creating a new instance.
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

  return { baseUrlOptions, availableProviders };
}

// ---------------------------------------------------------------------------
// useProviderInitialValues - form initial values for draft vs saved
// ---------------------------------------------------------------------------

/**
 * Build the form's initial values from the persisted instance, the
 * lazy-loaded `showProviderInstance` details, and the provider's default
 * base URL.
 *
 * - Draft: empty form, with `base_url` / `api_base` pre-filled from the
 *   provider's `default` URL when available.
 * - Saved: prefer `instanceDetails` (which carries api_key / base_url);
 *   normalise api_key into the bare key plus any nested credential
 *   fields (see {@link unwrapApiKey}).
 */
export function useProviderInitialValues(
  instance: IProviderInstance,
  instanceDetails: IProviderInstance | undefined,
  isDraft: boolean,
  baseUrlOptions: SelectOption[] | undefined,
) {
  return useMemo(() => {
    const defaultBaseUrl = pickDefaultUrl(baseUrlOptions);

    if (isDraft) {
      const values: Record<string, any> = { instance_name: '' };
      if (defaultBaseUrl) {
        values.base_url = defaultBaseUrl;
        values.api_base = defaultBaseUrl;
      }
      return values;
    }

    const merged: IProviderInstance = {
      ...instance,
      ...(instanceDetails ?? ({} as IProviderInstance)),
    };
    const values: Record<string, any> = {
      instance_name: merged.instance_name,
    };
    // api_key may come back as a JSON string, an already-parsed object,
    // or a plain bare key (see `unwrapApiKey`). Normalise it so the
    // api_key text field shows the bare key and the nested credential
    // fields (MiniMax group_id, Azure api_version, OpenRouter
    // provider_order) pre-fill their own form inputs.
    if (merged.api_key) {
      const { apiKey, nested } = unwrapApiKey(merged.api_key);
      values.api_key = apiKey;
      Object.assign(values, nested);
    }
    if (merged.base_url) {
      values.base_url = merged.base_url;
      values.api_base = merged.base_url;
    } else if (defaultBaseUrl) {
      values.base_url = defaultBaseUrl;
      values.api_base = defaultBaseUrl;
    }
    // The /providers/<p>/instances/<i> endpoint also returns `region`
    // for providers where it applies; surface it so the form / region
    // submit logic can echo it back.
    if ((merged as any).region) values.region = (merged as any).region;
    // Fallback: some backends may still surface these credential fields
    // at the top level rather than nested in api_key - echo any that
    // weren't already lifted from the api_key object above.
    for (const field of API_KEY_NESTED_FIELDS) {
      if (values[field] === undefined && (merged as any)[field] !== undefined) {
        values[field] = (merged as any)[field];
      }
    }
    return values;
  }, [instance, instanceDetails, isDraft, baseUrlOptions]);
}

// ---------------------------------------------------------------------------
// useLazyInstanceDetails - fetch full instance details when card opens
// ---------------------------------------------------------------------------

/**
 * The list endpoint (`useFetchProviderInstances`) does not return
 * sensitive/heavy fields like `api_key` or `base_url`. Pull the full
 * instance via `showProviderInstance` so the form can be pre-filled when
 * the user clicks an existing provider on the left. The hook is
 * `enabled: false` by default - we trigger it manually here so we
 * don't change behavior of other call sites.
 */
export function useLazyInstanceDetails(
  providerName: string,
  instanceName: string,
  isDraft: boolean,
  open: boolean,
) {
  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instanceName,
    );

  useEffect(() => {
    if (!isDraft && open && providerName && instanceName) {
      refetchInstanceDetails();
    }
  }, [isDraft, open, providerName, instanceName, refetchInstanceDetails]);

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
 *  (`values.api_key` / `values.base_url ?? values.api_base`) is used. */
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
 */
export function useVerifyProvider(
  providerName: string,
  formRef: RefObject<DynamicFormRef>,
  verifyTransform?: VerifyTransform,
) {
  const { verifyProviderConnection } = useVerifyProviderConnection();

  return useCallback(
    async (params: any) => {
      const values = { ...(formRef.current?.getValues?.() ?? {}), ...params };
      let verifyArgs: {
        api_key: string | object;
        base_url?: string;
        model_info?: IModelInfo[];
        region?: string;
      };
      if (verifyTransform) {
        const transformed = verifyTransform(values);
        verifyArgs = {
          api_key: transformed.apiKey,
          base_url: transformed.baseUrl,
          model_info: transformed.modelInfo ?? values.model_info,
          region: transformed.region,
        };
      } else {
        verifyArgs = {
          api_key: values.api_key ?? '',
          base_url: values.base_url ?? values.api_base,
          model_info: values.model_info,
        };
      }
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        ...(verifyArgs as any),
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
    [providerName, formRef, verifyProviderConnection, verifyTransform],
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
 *   - `markModelsEdited()`: absorb a model PATCH into the baseline so
 *     the next top-save does not re-PUT the same model_info (the PATCH
 *     endpoint already persisted it).
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
  const buildPayload = useCallback((): Record<string, any> | null => {
    const values = (formRef.current?.getValues?.() ?? {}) as Record<
      string,
      any
    >;
    const modelInfo =
      modelInfoRef.current.length > 0 ? modelInfoRef.current : [];

    // Provider-specific field mapping (e.g. OpenDataLoader's nested
    // `opendataloader_apiserver` / `opendataloader_api_key`). The
    // transform produces the canonical submit body shape
    // (`instance_name`, `llm_factory`, `api_key`, `api_base`,
    // `model_info`); we then layer on the card's own state (typed /
    // edited name, model_info ref, update-only fields for saved cards).
    if (submitTransform) {
      const transformed = submitTransform({
        ...values,
        model_info: modelInfo,
      }) as Record<string, any>;
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
      const resolvedId = instanceDetails?.id || instanceId;
      return {
        ...transformed,
        provider_name: providerName,
        instance_name: editedNameRef.current,
        id: resolvedId,
        base_url: transformed.base_url ?? transformed.api_base ?? '',
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
        instance_name: trimmed,
        api_key: buildApiKeyValue(values) ?? '',
        base_url: values.base_url ?? values.api_base,
      };
      if (modelInfoRef.current.length > 0) {
        payload.model_info = modelInfoRef.current;
      }
      return payload;
    }

    const resolvedId = instanceDetails?.id || instanceId;
    const apiKeyValue = buildApiKeyValue(values);
    const payload: Record<string, any> = {
      provider_name: providerName,
      // Use the edited name (may differ from the persisted `instanceName`
      // when the user has renamed the instance via double-click).
      instance_name: editedNameRef.current,
      id: resolvedId,
      api_key: apiKeyValue ?? '',
      base_url: values.base_url ?? values.api_base,
      region: values.region || 'default',
      model_info: modelInfoRef.current.length > 0 ? modelInfoRef.current : [],
      verify: false,
    };
    return payload;
  }, [
    isDraft,
    providerName,
    instanceName,
    instanceId,
    instanceDetails?.id,
    formRef,
    modelInfoRef,
    submitTransform,
  ]);

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
    const baseline = {
      provider_name: providerName,
      instance_name: instanceName,
      id: resolvedId,
      api_key: buildApiKeyValue(initialValues),
      base_url: initialValues.base_url ?? initialValues.api_base,
      region: initialValues.region,
      // model_info baseline is `[]`; `markModelsEdited` rewrites it after
      // a model PATCH so the next top-save short-circuits. The first
      // save after the models initially load may re-send the same
      // model_info (idempotent) - acceptable, matches the old
      // `lastSavedPayloadRef` seeding behaviour.
      model_info: [] as IModelInfo[],
      verify: false,
    };
    baselinePayloadRef.current = JSON.stringify(baseline);
  }, [
    isDraft,
    providerName,
    instanceName,
    instanceId,
    instanceDetails?.id,
    initialValues,
  ]);

  // `getSavePayload()` is the imperative entry point the parent calls
  // when the user clicks the top Save button. For drafts it always
  // returns a payload (provided the name is non-empty); for saved
  // cards it returns `null` when the current signature matches the
  // baseline, so the parent skips the no-op PUT.
  const getSavePayload = useCallback((): InstanceSavePayload | null => {
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
  }, [buildPayload, isDraft, instanceName]);

  // After a successful save the parent calls `markSaved()` so the
  // baseline catches up to the just-persisted values. Without this,
  // the next `getSavePayload()` would re-fire the same PUT.
  const markSaved = useCallback(() => {
    const payload = buildPayload();
    if (payload) {
      baselinePayloadRef.current = JSON.stringify(payload);
    }
  }, [buildPayload]);

  // Absorb a model patch into the baseline. `patchInstanceModel` has
  // already persisted the new max_tokens / model_type / features
  // server-side, so the next top-save should NOT re-PUT the same
  // model_info. By parsing the previously-saved baseline and overwriting
  // ONLY model_info, the baseline now matches the current state and the
  // signature check in `getSavePayload` short-circuits - while any
  // in-flight edits to api_key / base_url / region remain in the
  // baseline unchanged and will still trigger a save via signature
  // mismatch.
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
  _isDraft: boolean,
  initialValues: Record<string, any>,
  baseUrlOptions: SelectOption[] | undefined,
  hideWhenInstanceExists: (values: any) => boolean,
) {
  const { fields, defaultValues } = useProviderFields({
    llmFactory: providerName,
    // Always seed initial values (drafts need the default base_url;
    // saved cards need the persisted api_key / base_url).
    editMode: true,
    // Never disable fields - the old name-first lock is gone. Drafts
    // are fully editable from the start; saved cards are edited via the
    // top Save button.
    viewMode: false,
    initialValues,
    baseUrlOptions,
    hideWhenInstanceExists,
  });

  const formFields = useMemo(
    () => fields.filter((f) => f.name !== 'instance_name'),
    [fields],
  );

  const defaultValuesKey = JSON.stringify(defaultValues);
  const formDefaultValues = useMemo(() => {
    const { instance_name: _ignored, ...rest } = (defaultValues ??
      {}) as Record<string, any>;
    void _ignored;
    return rest;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultValuesKey]);

  return { formFields, formDefaultValues };
}

// Re-export ApiKeyNestedField for components that need it.
export type { ApiKeyNestedField };
