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
import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useAddProviderInstance,
  useDeleteProviderInstance,
  useFetchAvailableProviders,
  useFetchProviderInstance,
  useUpdateProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
  VerifyResult,
} from '../hooks';
import { useProviderFields } from '../provider-schema/hooks';
import { BedrockInstanceCard } from './bedrock-instance-card';
import { ModelsSection } from './models-section';
import VerifyButton from './verify-button';

interface ProviderInstanceCardProps {
  providerName: string;
  /**
   * The instance to render. When `isDraft` is true, this is a placeholder
   * used to render the "new instance" inline form; the actual save call
   * will use the values typed in the form fields.
   */
  instance: IProviderInstance;
  /**
   * True when this card represents a freshly-added (unsaved) instance.
   * Renders Save / Cancel buttons and treats all fields as editable.
   */
  isDraft?: boolean;
  /** Called after a draft instance is successfully saved. */
  onSaved?: (values: Record<string, any>) => void | Promise<void>;
  /**
   * Called after a draft instance's *name* has been persisted via
   * `addProviderInstance` (with just `instance_name`). The parent should
   * remove this draft from its visible list; the freshly invalidated
   * `providerInstances` query will surface the persisted card.
   */
  onNameSaved?: () => void;
  /**
   * Called when the user deletes a draft instance.
   * For drafts this is equivalent to onCancel; for saved instances
   * the component calls useDeleteProviderInstance internally.
   */
  onDelete?: () => void;
  /**
   * When true, this card starts expanded and its instance details
   * are fetched on mount. Default `false` so additional cards stay
   * collapsed until the user opens them — at which point details
   * are fetched on demand.
   */
  defaultOpen?: boolean;
}

/**
 * One inline provider-instance card. The provider name + doc-link arrow
 * live in the parent page's sticky `ProviderHeaderBar`; this card only
 * shows the **instance**-level details (name, fields, verify, models).
 *
 * Two visual modes (driven by the `nameSaved` flag, not the `isDraft`
 * prop — `isDraft` only controls whether the form is editable):
 *  1. **Unsaved name** (`!nameSaved`): the instance name lives in a
 *     dedicated form-field section at the top of the body, wrapped in
 *     a red border with a label, input, inline Save button, and
 *     always-visible helper text. The form fields are always visible
 *     (no collapsible). The auto-save on blur is *active* but will
 *     refuse to call `onSaved` until the name is entered and saved.
 *  2. **Saved name** (`nameSaved`): the form-field section collapses
 *     into a single collapsible row showing the name as plain text
 *     with a hover-only key/lock icon. The form fields live inside
 *     the collapsible content and can be collapsed/expanded.
 */
export function ProviderInstanceCard(props: ProviderInstanceCardProps) {
  // AWS Bedrock has provider-specific fields (auth_mode, region, AK/SK,
  // role ARN, model name, max_tokens) that don't fit the generic
  // DynamicForm path. Render its own inline card instead.
  //
  // Dispatch BEFORE any hooks so each branch component has a stable
  // hook-call order (Rules of Hooks).
  if (props.providerName === 'Bedrock') {
    return <BedrockInstanceCard {...props} />;
  }
  return <GenericProviderInstanceCard {...props} />;
}

function GenericProviderInstanceCard({
  providerName,
  instance,
  isDraft = false,
  onSaved,
  onNameSaved,
  onDelete,
  defaultOpen = false,
}: ProviderInstanceCardProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  // Drafts always start open (the user just added them and needs to
  // fill the fields); saved cards default to collapsed unless the
  // parent flagged this card as the one to expand initially (typically
  // the first instance in the list).
  const [open, setOpen] = useState(isDraft || defaultOpen);
  // Drafts start with an empty name — the user types it themselves.
  const [draftName, setDraftName] = useState('');
  // Tracks whether the instance name has been saved for the current
  // draft/saved state. Saved instances start with `true` (the name is
  // persisted in the backend); draft instances start with `false` and
  // flip to `true` after the dedicated "Save" button on the name
  // section is pressed.
  const [nameSaved, setNameSaved] = useState(!isDraft);
  const formRef = useRef<DynamicFormRef>(null);
  // Guards against concurrent auto-save calls: while one save is in
  // flight, additional form changes shouldn't trigger another `onSaved`.
  const savingRef = useRef(false);

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
      setNameSaved(false);
    } else {
      setNameSaved(true);
    }
  }, [providerName, isDraft]);

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

  // Auto-save: whenever the form's other fields change (in draft mode),
  // watch the form values and, after a 200ms debounce (acting as a blur
  // proxy — fires shortly after the user stops typing / blurs out of a
  // field), trigger validation. If all required fields are valid AND
  // the instance name has been entered and saved, call `onSaved` with
  // the merged values. The name check happens in the handler (not as an
  // early-return gate) so the auto-save can be observed firing even
  // when the name is missing — only the actual `onSaved` call is
  // suppressed.
  useEffect(() => {
    if (!isDraft) return;

    const formInstance = (formRef.current as any)?.form;
    if (!formInstance || typeof formInstance.watch !== 'function') return;

    let saveTimeout: ReturnType<typeof setTimeout> | null = null;
    let cancelled = false;

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
          await onSavedRef.current({
            ...values,
            instance_name: draftNameRef.current.trim(),
          });
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
  }, [isDraft, nameSaved]);

  const { instanceNameSet } = useFetchInstanceNameSet(
    isDraft ? providerName : '',
  );
  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);

  // Fetch the catalog of available providers so we can pre-fill the
  // `base_url` / `api_base` field with the provider's default URL
  // (e.g. `https://api.openai.com/v1`) when creating a new instance.
  // Only used in draft mode; saved instances carry their own URL.
  const { data: availableProviders } = useFetchAvailableProviders();

  // For saved instances, the list endpoint (`useFetchProviderInstances`)
  // does not return sensitive/heavy fields like `api_key` or `base_url`.
  // Pull the full instance via `showProviderInstance` so the form can be
  // pre-filled when the user clicks an existing provider on the left.
  // The hook is `enabled: false` by default — we trigger it manually
  // here so we don't change behavior of other call sites.
  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instance.instance_name,
    );

  // Lazily fetch full instance details only when the card is open
  // (or pre-opened via defaultOpen). Cards that stay collapsed never
  // hit /providers/<name>/instances/<instance_name>. Each expand
  // triggers a refetch so the user always sees fresh values.
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

  // Build the form fields from the provider config. In draft mode we don't
  // pass any initial values; otherwise we pre-fill the form with the
  // instance's stored fields, preferring the full `showProviderInstance`
  // payload (which includes api_key/base_url) over the list-level row.
  const initialValues = useMemo(() => {
    if (isDraft) {
      return { instance_name: '' };
    }
    const merged: IProviderInstance = {
      ...instance,
      ...(instanceDetails ?? ({} as IProviderInstance)),
    };
    const values: Record<string, any> = {
      instance_name: merged.instance_name,
    };
    if (merged.api_key) values.api_key = merged.api_key;
    if (merged.base_url) {
      values.base_url = merged.base_url;
      values.api_base = merged.base_url;
    }
    // The /providers/<p>/instances/<i> endpoint also returns `region`
    // for providers where it applies; surface it so the form / region
    // submit logic can echo it back.
    if ((merged as any).region) values.region = (merged as any).region;
    return values;
  }, [instance, instanceDetails, isDraft]);

  const buildBaseUrlOptions = useCallback(
    (urlObj?: Record<string, string | undefined>) => {
      if (!urlObj) return undefined;
      const options = Object.keys(urlObj)
        .filter((k) => !!urlObj[k])
        .map((k) => {
          const v = urlObj[k] as string;
          // if (k === 'default') {
          //   return { value: v, label: v };
          // }
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
    },
    [],
  );
  const currentProvider = useMemo(
    () =>
      providerName
        ? availableProviders.find((p) => p.name === providerName)
        : undefined,
    [availableProviders, providerName],
  );
  const baseUrlOptions = useMemo(
    () => buildBaseUrlOptions(currentProvider?.url),
    [buildBaseUrlOptions, currentProvider],
  );

  const { fields, defaultValues } = useProviderFields({
    llmFactory: providerName,
    editMode: !isDraft,
    viewMode: isDraft,
    initialValues,
    baseUrlOptions,
    hideWhenInstanceExists,
  });

  // The card header owns the instance name (editable on hover). Drop
  // the `instance_name` field from the form so the user does not see
  // it twice and we keep a single source of truth.
  const formFields = useMemo(
    () => fields.filter((f) => f.name !== 'instance_name'),
    [fields],
  );
  const formDefaultValues = useMemo(() => {
    const { instance_name: _ignored, ...rest } = (defaultValues ??
      {}) as Record<string, any>;
    void _ignored;
    return rest;
  }, [defaultValues]);

  // When the lazy `showProviderInstance` fetch resolves (or refetches
  // after the user collapses + re-expands), `formDefaultValues` will
  // pick up the new api_key / base_url / region. React-Hook-Form only
  // consumes `defaultValues` on first mount, so we explicitly reset
  // the form here to make the freshly-fetched values visible. We use
  // `keepDirtyValues` so the user's in-progress edits (if any) are
  // not clobbered by a background refetch.
  useEffect(() => {
    if (isDraft) return;
    if (!instanceDetails) return;
    const form = (formRef.current as any)?.form;
    if (form?.reset) {
      form.reset(formDefaultValues, { keepDirtyValues: true });
    } else {
      formRef.current?.reset?.(formDefaultValues);
    }
  }, [isDraft, instanceDetails, formDefaultValues]);

  // Verify callback: just proxies the form values through. The VerifyButton
  // re-uses the existing shared verify hook; the modal-style verify flow
  // (verifyTransform etc.) is intentionally not invoked here.
  const { verifyProviderConnection } = useVerifyProviderConnection();
  const handleVerify = useCallback(
    async (params: any) => {
      const values = { ...(formRef.current?.getValues?.() ?? {}), ...params };
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: values.api_key ?? '',
        base_url: values.base_url ?? values.api_base,
        model_info: values.model_info,
      });
      if (ret.code === 0) {
        return { isValid: true, logs: ret.message } as VerifyResult;
      }
      return { isValid: false, logs: ret.message } as VerifyResult;
    },
    [providerName, verifyProviderConnection],
  );

  // Save the instance name on its own. Calls addProviderInstance with
  // only the instance name (backend now supports creating an instance
  // with just a name). On success notifies the parent via onNameSaved
  // so it can remove this draft — the invalidated providerInstances
  // query will surface the persisted card automatically.
  const { addProviderInstance } = useAddProviderInstance();
  const handleSaveName = useCallback(async () => {
    const trimmed = draftName.trim();
    if (!trimmed) return;
    const ret = await addProviderInstance({
      llm_factory: providerName,
      instance_name: trimmed,
    } as any);
    if (ret?.code === 0) {
      onNameSaved?.();
    }
  }, [draftName, addProviderInstance, providerName, onNameSaved]);

  // ── Blur-driven auto-save for saved (non-draft) cards ───────────────
  // For persisted instances the user edits non-name fields (api_key,
  // base_url, region, ...) and we save automatically when a field
  // loses focus via the dedicated PUT endpoint:
  //   PUT /api/v1/providers/<provider_name>/instances/<instance_name>
  // Both `id` and `instance_name` are sent in the body but the backend
  // rejects any change to them — they are echoed back unchanged so the
  // backend can locate the row.
  const { updateProviderInstance } = useUpdateProviderInstance();
  const blurSavingRef = useRef(false);
  // Flipped to true while a child (e.g. ModelsSection's
  // AddCustomModelDialog) is rendering a Portal-based dialog. The dialog
  // body is outside this card's `onBlurCapture` container, so opening it
  // would otherwise fire a spurious blur-save. The child notifies us via
  // `onBlurSuppressChange`.
  const blurSuppressRef = useRef(false);
  const lastSavedPayloadRef = useRef<string>('');
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
      if (blurSavingRef.current) return;
      if (blurSuppressRef.current) return;

      const isValid = await formRef.current?.trigger();
      if (!isValid) return;

      const values = formRef.current?.getValues?.() ?? {};
      const instanceId = instanceDetails?.id || instance.id;
      const payload = {
        provider_name: providerName,
        instance_name: instance.instance_name,
        id: instanceId,
        api_key: values.api_key,
        base_url: values.base_url ?? values.api_base,
        region: values.region,
        model_info: values.model_info,
      };
      // Skip if nothing actually changed since the last save (or initial
      // mount): prevents a no-op PUT on every focus shift.
      const signature = JSON.stringify(payload);
      if (signature === lastSavedPayloadRef.current) return;

      blurSavingRef.current = true;
      try {
        const ret = await updateProviderInstance(payload);
        if (ret?.code === 0) {
          lastSavedPayloadRef.current = signature;
        }
      } finally {
        blurSavingRef.current = false;
      }
    },
    [
      isDraft,
      providerName,
      instance.instance_name,
      instance.id,
      instanceDetails?.id,
      updateProviderInstance,
    ],
  );

  // Seed the "last saved" signature once initial values are loaded so the
  // first blur after mount doesn't trigger an unnecessary save.
  useEffect(() => {
    if (isDraft) return;
    const instanceId = instanceDetails?.id || instance.id;
    if (!instanceId) return;
    const baseline = {
      provider_name: providerName,
      instance_name: instance.instance_name,
      id: instanceId,
      api_key: initialValues.api_key,
      base_url: initialValues.base_url ?? initialValues.api_base,
      region: initialValues.region,
      model_info: undefined,
    };
    lastSavedPayloadRef.current = JSON.stringify(baseline);
  }, [
    isDraft,
    providerName,
    instance.instance_name,
    instance.id,
    instanceDetails?.id,
    initialValues,
  ]);

  // Delete handler: for saved instances calls useDeleteProviderInstance;
  // for drafts calls onDelete (which maps to onCancel in the parent).
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

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {nameSaved ? (
        // ── SAVED MODE ───────────────────────────────────────────────
        // The name is locked. Render it as a plain-text row that acts
        // as the collapsible trigger. The form fields (API-Key,
        // Base-Url, Verify, Models) live inside the collapsible
        // content and can be collapsed/expanded via the chevron.
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger asChild>
            <div className="flex items-center gap-1 w-full mb-5">
              <div
                className="group flex items-center flex-1 gap-2 px-2 py-1 cursor-pointer bg-bg-input rounded-md"
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
          <CollapsibleContent forceMount className="data-[state=closed]:hidden">
            <div
              className="pb-4 flex flex-col gap-4"
              onBlurCapture={handleFieldsBlur}
            >
              <DynamicForm.Root
                key={`${providerName}-${instance.instance_name}-${isDraft}-${instanceDetails ? 'loaded' : 'pending'}`}
                ref={formRef}
                fields={formFields}
                onSubmit={() => undefined}
                defaultValues={formDefaultValues}
                labelClassName="font-normal"
              />

              <div className=" pt-3">
                <VerifyButton
                  onVerify={handleVerify}
                  isAbsolute={false}
                  formRef={formRef}
                />
              </div>

              {/*
                Only mount ModelsSection when the card is expanded.
                CollapsibleContent uses `forceMount` so the form above stays
                in the DOM across open/close transitions, but we do NOT want
                the per-instance models query (and the upstream catalog
                fetch) to fire for every collapsed sibling — that would
                produce one request per instance every time the provider is
                clicked. Gating on `open` here ensures only the expanded
                instance hits /api/v1/providers/<p>/instances/<i>/models.
              */}
              {open && (
                <div className=" pt-3">
                  <ModelsSection
                    providerName={providerName}
                    instanceName={instance.instance_name || '__draft__'}
                    instance={instance}
                    hideActions={false}
                    hideIfEmpty={false}
                    getFormValues={() => formRef.current?.getValues?.() ?? {}}
                    onBlurSuppressChange={(s) => {
                      blurSuppressRef.current = s;
                    }}
                  />
                </div>
              )}
            </div>
          </CollapsibleContent>
        </Collapsible>
      ) : (
        // ── UNSAVED MODE ─────────────────────────────────────────────
        // The name is in a dedicated form-field section. The input
        // itself carries the destructive red border (no wrapping red
        // box). The section is always visible (no collapsible) so the
        // user can see the warning and the helper text. The form
        // fields (API-Key, Base-Url, Verify, Models) follow below the
        // name section.
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
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleSaveName();
                  }
                }}
                // The input itself carries the red border (not a
                // wrapping box). Persists while the name is unsaved.
                className="flex-1  rounded-r-none"
                data-testid="instance-name-input"
              />
              <Button
                onClick={handleSaveName}
                disabled={!draftName.trim()}
                data-testid="instance-name-save"
                variant="outline"
                className="rounded-l-none bg-bg-input shrink-0"
              >
                {tSetting('save')}
              </Button>
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
            <p
              className="text-xs text-text-secondary"
              data-testid="instance-name-helper"
            >
              {tSetting('instanceNameSaveTip')}
            </p>
          </div>

          <fieldset
            disabled={!nameSaved}
            className="contents disabled:[&_*]:pointer-events-none disabled:opacity-60"
            data-testid="instance-locked-fields"
          >
            <DynamicForm.Root
              key={`${providerName}-${instance.instance_name}-${isDraft}`}
              ref={formRef}
              fields={formFields}
              onSubmit={() => undefined}
              defaultValues={formDefaultValues}
              labelClassName="font-normal"
            />

            <div className=" pt-3">
              <VerifyButton
                onVerify={handleVerify}
                isAbsolute={false}
                formRef={formRef}
              />
            </div>

            <div className=" pt-3">
              <ModelsSection
                providerName={providerName}
                instanceName={instance.instance_name || '__draft__'}
                instance={instance}
                hideActions={false}
                hideIfEmpty={false}
                getFormValues={() => formRef.current?.getValues?.() ?? {}}
              />
            </div>
          </fieldset>

          {/* {isDraft && (
            <div className="flex items-center justify-end gap-2 border-t border-border-button pt-3">
              <Button
                variant="outline"
                onClick={onCancel}
                data-testid="draft-cancel"
              >
                {tSetting('cancel')}
              </Button>
            </div>
          )} */}
        </div>
      )}
    </div>
  );
}

export default ProviderInstanceCard;
