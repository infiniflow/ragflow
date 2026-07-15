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
  useAddProviderInstance,
  useDeleteProviderInstance,
  useFetchAvailableProviders,
  useFetchProviderInstance,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import {
  IModelInfo,
  IUpdateProviderInstanceRequestBody,
} from '@/interfaces/request/llm';
import { RefObject, useCallback, useEffect, useMemo, useRef } from 'react';
import { useProviderFields } from '../provider-schema/hooks';
import { SelectOption } from '../provider-schema/types';
import { API_KEY_NESTED_FIELDS, ApiKeyNestedField } from './interface';

// ---------------------------------------------------------------------------
// Pure helpers — api_key shape normalization
// ---------------------------------------------------------------------------

/**
 * Build the `api_key` payload value from the flat form values. If any of
 * the nested credential fields (see `API_KEY_NESTED_FIELDS`) carry a
 * value, returns `{ api_key, ...nested }`; otherwise returns the plain
 * api_key string. Used by both the auto-save payload and its change
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
// useProviderBaseUrlOptions — fetch provider catalog and build URL options
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
// useProviderInitialValues — form initial values for draft vs saved
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
    // at the top level rather than nested in api_key — echo any that
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
// useLazyInstanceDetails — fetch full instance details when card opens
// ---------------------------------------------------------------------------

/**
 * The list endpoint (`useFetchProviderInstances`) does not return
 * sensitive/heavy fields like `api_key` or `base_url`. Pull the full
 * instance via `showProviderInstance` so the form can be pre-filled when
 * the user clicks an existing provider on the left. The hook is
 * `enabled: false` by default — we trigger it manually here so we
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
// useFormResetOnDetailsLoad — re-fill form when instanceDetails resolves
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
// useVerifyProvider — wraps useVerifyProviderConnection for the card
// ---------------------------------------------------------------------------

/**
 * Adapter that reads the current form values and proxies them into
 * `verifyProviderConnection`, then shapes the response into the
 * `VerifyResult` consumed by {@link VerifyButton}.
 */
export function useVerifyProvider(
  providerName: string,
  formRef: RefObject<DynamicFormRef>,
) {
  const { verifyProviderConnection } = useVerifyProviderConnection();

  return useCallback(
    async (params: any) => {
      const values = { ...(formRef.current?.getValues?.() ?? {}), ...params };
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: values.api_key ?? '',
        base_url: values.base_url ?? values.api_base,
        model_info: values.model_info,
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
    [providerName, formRef, verifyProviderConnection],
  );
}

// ---------------------------------------------------------------------------
// useSaveInstanceName — dedicated hook for the draft name Save button
// ---------------------------------------------------------------------------

/**
 * Save the instance name on its own. Calls `addProviderInstance` with
 * only the instance name (backend now supports creating an instance with
 * just a name). On success notifies the parent via `onNameSaved` so it
 * can remove this draft — the invalidated `providerInstances` query
 * will surface the persisted card automatically.
 */
export function useSaveInstanceName(
  providerName: string,
  draftName: string,
  onNameSaved?: (instanceName: string) => void,
) {
  const { addProviderInstance } = useAddProviderInstance();
  return useCallback(async () => {
    const trimmed = draftName.trim();
    if (!trimmed) return;
    const ret = await addProviderInstance({
      llm_factory: providerName,
      instance_name: trimmed,
    } as any);
    if (ret?.code === 0) {
      onNameSaved?.(trimmed);
    }
  }, [draftName, addProviderInstance, providerName, onNameSaved]);
}

// ---------------------------------------------------------------------------
// useDeleteInstance — wires the card's delete button
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
// useDraftAutoSave — 200ms-debounced watch-based save for draft mode
// ---------------------------------------------------------------------------

/**
 * Auto-save: whenever the form's other fields change (in draft mode),
 * watch the form values and, after a 200ms debounce (acting as a blur
 * proxy — fires shortly after the user stops typing / blurs out of a
 * field), trigger validation. If all required fields are valid AND
 * the instance name has been entered and saved, call `onSaved` with
 * the merged values.
 */
export function useDraftAutoSave(
  formRef: RefObject<DynamicFormRef>,
  isDraft: boolean,
  nameSaved: boolean,
  draftName: string,
  onSaved: ((values: Record<string, any>) => void | Promise<void>) | undefined,
  modelInfoRef: { current: IModelInfo[] },
) {
  // Keep the latest `onSaved` and `draftName` in refs so the auto-save
  // effect below can read them without re-subscribing on every render
  // (the parent passes a fresh `onSaved` arrow each render).
  const onSavedRef = useRef(onSaved);
  useEffect(() => {
    onSavedRef.current = onSaved;
  });
  const draftNameRef = useRef(draftName);
  useEffect(() => {
    draftNameRef.current = draftName;
  });

  useEffect(() => {
    if (!isDraft) return;

    const formInstance = (formRef.current as any)?.form;
    if (!formInstance || typeof formInstance.watch !== 'function') return;

    let saveTimeout: ReturnType<typeof setTimeout> | null = null;
    let cancelled = false;
    const savingRef = { current: false };

    const subscription = formInstance.watch(() => {
      if (saveTimeout) clearTimeout(saveTimeout);
      saveTimeout = setTimeout(async () => {
        if (cancelled || savingRef.current) return;
        const isValid = await formRef.current?.trigger();
        if (cancelled || savingRef.current) return;
        if (!isValid) return;

        // Name gate: refuse to actually save if the name is empty or
        // has not been "saved" (locked). The red border on the name
        // section is the visible signal — it stays on while
        // `!nameSaved` regardless of whether the user has touched
        // other fields.
        if (!draftNameRef.current.trim() || !nameSaved) return;
        if (!onSavedRef.current) return;

        savingRef.current = true;
        try {
          const values = formRef.current?.getValues?.() ?? {};
          const payload: Record<string, any> = {
            ...values,
            instance_name: draftNameRef.current.trim(),
          };
          // Forward the latest per-instance model list as `model_info`
          // when the user has attached models to the draft. The list
          // is normally empty for drafts (the backend has nothing yet),
          // but it can be populated once ModelsSection has fetched /
          // received data — we only set the key when non-empty so an
          // absent value is preserved as `undefined` rather than
          // forcing an empty-array clear on the server.
          if (modelInfoRef.current.length > 0) {
            payload.model_info = modelInfoRef.current;
          }
          await onSavedRef.current(payload);
        } finally {
          savingRef.current = false;
        }
      }, 200);
    });

    return () => {
      cancelled = true;
      if (saveTimeout) clearTimeout(saveTimeout);
      try {
        subscription?.unsubscribe?.();
      } catch {
        // ignore cleanup errors
      }
    };
  }, [isDraft, nameSaved, formRef, modelInfoRef]);
}

// ---------------------------------------------------------------------------
// useSavedAutoSave — blur + dropdown-driven auto-save for saved cards
// ---------------------------------------------------------------------------

/** Field types whose value is committed via click/select (not blur). The
 *  card's `onBlurCapture` auto-save fires before the dropdown click
 *  handler commits the new value, and the popover content is rendered in
 *  a Radix portal outside the card's blur container, so blur-based saves
 *  are unreliable for these. We watch the form values directly and
 *  trigger the same auto-save on value change. */
const DROPDOWN_FIELD_TYPES = new Set<FormFieldType>([
  FormFieldType.Select,
  FormFieldType.MultiSelect,
  FormFieldType.Segmented,
  // `Custom` is the form-field type used by `inputSelect` in this
  // codebase (see use-provider-fields). Every `Custom` field rendered
  // inside the provider instance card is an `InputSelect` dropdown.
  FormFieldType.Custom,
]);

interface UseSavedAutoSaveArgs {
  formRef: RefObject<DynamicFormRef>;
  formFields: Array<{ name: string; type: FormFieldType; [k: string]: any }>;
  providerName: string;
  instanceName: string;
  instanceId: string | undefined;
  isDraft: boolean;
  instanceDetails: IProviderInstance | undefined;
  initialValues: Record<string, any>;
  modelInfoRef: { current: IModelInfo[] };
}

/**
 * Wires the blur-driven + dropdown-driven auto-save flow used by saved
 * (non-draft) cards. Returns a `handleFieldsBlur` for the card body to
 * attach to `onBlurCapture`, plus a `markModelsEdited` callback for
 * `onInstanceModelsEdited` so the next auto-save short-circuits after
 * a PATCH-driven model change.
 */
export function useSavedAutoSave({
  formRef,
  formFields,
  providerName,
  instanceName,
  instanceId,
  isDraft,
  instanceDetails,
  initialValues,
  modelInfoRef,
}: UseSavedAutoSaveArgs) {
  const { updateProviderInstance } = useUpdateProviderInstance();
  const blurSavingRef = useRef(false);
  const blurSuppressRef = useRef(false);
  const lastSavedPayloadRef = useRef<string>('');
  const hasSyncedInstanceRef = useRef(false);
  const autoSaveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const AUTO_SAVE_DEBOUNCE_MS = 500;

  // Shared auto-save routine. Triggered by:
  //   - `handleFieldsBlur` (focus leaves a non-dropdown field), and
  //   - the dropdown value-change watcher (a dropdown field's value
  //     commits via click, not blur — and the popover is rendered in a
  //     Radix portal outside the card's blur container, so blur-based
  //     saves are unreliable for dropdowns).
  const performAutoSave = useCallback(async () => {
    if (isDraft) return;
    if (blurSavingRef.current) return;
    if (blurSuppressRef.current) return;

    const isValid = await formRef.current?.trigger();
    if (!isValid) return;

    const values = formRef.current?.getValues?.() ?? {};
    const resolvedId = instanceDetails?.id || instanceId;
    // Providers like MiniMax / Azure-OpenAI / OpenRouter carry extra
    // credential fields (group_id / api_version / provider_order) that
    // the backend expects bundled *inside* api_key as an object rather
    // than as top-level keys. Nesting them here also folds their values
    // into the change signature below, so editing one actually triggers
    // a blur-save.
    const apiKeyValue = buildApiKeyValue(values as Record<string, any>);
    const payload: IUpdateProviderInstanceRequestBody = {
      provider_name: providerName,
      instance_name: instanceName,
      id: resolvedId,
      api_key: apiKeyValue ?? '',
      base_url: values.base_url ?? values.api_base,
      region: values.region || 'default',
      model_info: [],
      verify: false,
    };
    // Pull the latest model list from ModelsSection (via the ref it
    // updates). Only attach when non-empty so we don't accidentally
    // wipe the persisted model set with an empty array.
    if (modelInfoRef.current.length > 0) {
      payload.model_info = modelInfoRef.current;
    }
    // Skip if nothing actually changed since the last save (or initial
    // mount): prevents a no-op PUT on every focus shift.
    const signature = JSON.stringify(payload);
    if (signature === lastSavedPayloadRef.current) return;

    blurSavingRef.current = true;
    try {
      const ret = await updateProviderInstance(payload);
      if (ret?.code === 0) {
        lastSavedPayloadRef.current = signature;
        hasSyncedInstanceRef.current = true;
      }
    } finally {
      blurSavingRef.current = false;
    }
  }, [
    isDraft,
    providerName,
    instanceName,
    instanceId,
    instanceDetails?.id,
    updateProviderInstance,
    formRef,
    modelInfoRef,
  ]);

  // Debounced auto-save: coalesces rapid edits (blur cascade,
  // successive dropdown changes, typing in filterable dropdowns) into
  // a single delayed `performAutoSave` call.
  const scheduleAutoSave = useCallback(() => {
    if (isDraft) return;
    if (autoSaveTimeoutRef.current) {
      clearTimeout(autoSaveTimeoutRef.current);
    }
    autoSaveTimeoutRef.current = setTimeout(() => {
      autoSaveTimeoutRef.current = null;
      void performAutoSave();
    }, AUTO_SAVE_DEBOUNCE_MS);
  }, [isDraft, performAutoSave]);

  const handleFieldsBlur = useCallback(
    async (e: React.FocusEvent<HTMLDivElement>) => {
      if (isDraft) return;
      // Ignore focus moves that stay inside the same container.
      if (
        e.currentTarget.contains(e.relatedTarget as Node | null) &&
        e.relatedTarget !== null
      ) {
        return;
      }
      scheduleAutoSave();
    },
    [isDraft, scheduleAutoSave],
  );

  // Refs so the dropdown watcher effect can invoke the latest callbacks
  // without re-subscribing on every render (the parent passes a fresh
  // `onBlurCapture` arrow each render, and `performAutoSave` changes
  // whenever its deps change — e.g. when `instanceDetails` loads).
  const performAutoSaveRef = useRef(performAutoSave);
  useEffect(() => {
    performAutoSaveRef.current = performAutoSave;
  });
  const scheduleAutoSaveRef = useRef<() => void>(() => {});
  useEffect(() => {
    scheduleAutoSaveRef.current = scheduleAutoSave;
  });

  // Clear any pending debounced save when the card unmounts so we don't
  // fire a stale request after teardown.
  useEffect(() => {
    return () => {
      if (autoSaveTimeoutRef.current) {
        clearTimeout(autoSaveTimeoutRef.current);
        autoSaveTimeoutRef.current = null;
      }
    };
  }, []);

  // Seed the "last saved" signature once initial values are loaded so
  // the first blur after mount doesn't trigger an unnecessary save.
  useEffect(() => {
    if (isDraft) return;
    const resolvedId = instanceDetails?.id || instanceId;
    if (!resolvedId) return;
    if (hasSyncedInstanceRef.current) return;
    // Match the api_key shape performAutoSave produces (extra credential
    // fields nested inside api_key) so the first blur after mount
    // doesn't see a signature diff and fire a redundant save. model_info
    // is omitted for the same reason as in performAutoSave: model
    // changes are owned by the per-model endpoints, not this auto-save.
    const baseline = {
      provider_name: providerName,
      instance_name: instanceName,
      id: resolvedId,
      api_key: buildApiKeyValue(initialValues),
      base_url: initialValues.base_url ?? initialValues.api_base,
      region: initialValues.region,
      model_info: [] as IModelInfo[],
    };
    lastSavedPayloadRef.current = JSON.stringify(baseline);
  }, [
    isDraft,
    providerName,
    instanceName,
    instanceId,
    instanceDetails?.id,
    initialValues,
  ]);

  // Dropdown value-change auto-save (saved mode only). A dropdown
  // field's value commits via click, not blur — and the popover is
  // rendered in a Radix portal outside the card's blur container, so
  // blur-based saves are unreliable for dropdowns.
  //
  // We subscribe to the *raw* RHF form so we can read the change
  // metadata `{ name, type }`. Only genuine user edits carry
  // `type === 'change'`; programmatic updates (the `form.reset` that
  // runs when `instanceDetails` loads, `setValue`, etc.) come through
  // with an undefined type. Gating on `type === 'change'` is what stops
  // merely opening the card / loading its details from firing a save —
  // only an actual user selection in a dropdown schedules one. The
  // shared debounce (`scheduleAutoSave`) coalesces it with any
  // blur-triggered save. Skipped in draft mode (which has its own
  // 200ms-debounced watch) and until `instanceDetails` loads.
  const dropdownFieldNames = useMemo(
    () =>
      formFields
        .filter((f) => DROPDOWN_FIELD_TYPES.has(f.type))
        .map((f) => f.name),
    [formFields],
  );

  useEffect(() => {
    if (isDraft) return;
    if (dropdownFieldNames.length === 0) return;
    if (!instanceDetails) return;
    const form = (formRef.current as any)?.form;
    if (!form || typeof form.watch !== 'function') return;

    let cancelled = false;
    const dropdownFieldSet = new Set(dropdownFieldNames);

    const subscription = form.watch(
      (_values: any, meta: { name?: string; type?: string }) => {
        if (cancelled) return;
        // Ignore programmatic value changes (reset/setValue) — they have
        // no `type`. Only react to user-driven dropdown selections.
        if (meta?.type !== 'change') return;
        if (!meta?.name || !dropdownFieldSet.has(meta.name)) return;
        scheduleAutoSaveRef.current();
      },
    );

    return () => {
      cancelled = true;
      try {
        subscription?.unsubscribe?.();
      } catch {
        // ignore cleanup errors
      }
    };
  }, [isDraft, dropdownFieldNames, instanceDetails, formRef]);

  // Absorb a model patch into the host's last-saved baseline. When the
  // user saves the edit modal, patchInstanceModel has already persisted
  // the new max_tokens / model_type / features server-side, so the next
  // blur auto-save should NOT re-PUT the same model_info. By parsing
  // the previously-saved payload and overwriting ONLY model_info, the
  // baseline now matches the current state and the signature check in
  // performAutoSave short-circuits — while any in-flight edits to
  // api_key / base_url / region remain in `lastSavedPayloadRef`
  // unchanged and will still trigger a save on blur via signature
  // mismatch.
  //
  // Skipped until the host has synced at least once. Before that the
  // baseline still carries the initial `model_info: []`; rewriting it
  // here would skip the very first PUT that syncs the user's first
  // add/edit into the persisted model_info.
  const markModelsEdited = useCallback(() => {
    if (isDraft) return;
    if (!hasSyncedInstanceRef.current) return;
    const prev = lastSavedPayloadRef.current;
    if (!prev) return;
    const parsed = JSON.parse(prev) as IUpdateProviderInstanceRequestBody;
    parsed.model_info =
      modelInfoRef.current.length > 0 ? modelInfoRef.current : [];
    // Mirror the `verify: false` field that performAutoSave always
    // attaches, otherwise the next signature comparison would diff on
    // this key alone and re-fire the save.
    (parsed as any).verify = false;
    lastSavedPayloadRef.current = JSON.stringify(parsed);
  }, [isDraft, modelInfoRef]);

  return {
    handleFieldsBlur,
    performAutoSave,
    scheduleAutoSave,
    blurSuppressRef,
    markModelsEdited,
  };
}

// ---------------------------------------------------------------------------
// useFormFields — wraps useProviderFields and strips instance_name
// ---------------------------------------------------------------------------

/**
 * Wraps `useProviderFields` and removes the `instance_name` field from
 * both the field list and the default values — the card header owns
 * the instance name (editable on hover), so we keep a single source of
 * truth and avoid showing it twice in the form.
 */
export function useFormFields(
  providerName: string,
  isDraft: boolean,
  initialValues: Record<string, any>,
  baseUrlOptions: SelectOption[] | undefined,
  hideWhenInstanceExists: (values: any) => boolean,
) {
  const { fields, defaultValues } = useProviderFields({
    llmFactory: providerName,
    editMode: !isDraft,
    viewMode: isDraft,
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
