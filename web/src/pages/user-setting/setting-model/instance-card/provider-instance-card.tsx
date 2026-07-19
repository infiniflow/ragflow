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
import { IModelInfo } from '@/interfaces/request/llm';
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useFetchInstanceNameSet, useHideWhenInstanceExists } from '../hooks';
import { getProviderConfig } from '../provider-schema/field-config';
import { BedrockInstanceCard } from './bedrock-instance-card';
import { DraftModeCard } from './components/draft-mode-card';
import { InstanceNameSection } from './components/instance-name-section';
import { SavedModeCard } from './components/saved-mode-card';
import {
  useDeleteInstance,
  useFormFields,
  useFormResetOnDetailsLoad,
  useInstanceSaveState,
  useLazyInstanceDetails,
  useProviderBaseUrlOptions,
  useProviderInitialValues,
  useVerifyProvider,
} from './hooks';
import {
  ProviderInstanceCardProps,
  ProviderInstanceCardRef,
} from './interface';
import { SoMarkInstanceCard } from './somark-instance-card';

/**
 * One inline provider-instance card. The provider name + doc-link arrow
 * live in the parent page's sticky `ProviderHeaderBar`; this card only
 * shows the **instance**-level details (name, fields, verify, models).
 *
 * Two visual modes driven by `isDraft`:
 *  1. **Draft** (`isDraft`): the instance name lives in a dedicated
 *     input section at the top of the body, and the form fields are
 *     always editable (no fieldset lock). The parent drives save
 *     through the imperative ref API.
 *  2. **Saved** (`!isDraft`): the form-field section collapses into a
 *     single collapsible row showing the name as plain text with a
 *     hover-only key/lock icon. The form fields live inside the
 *     collapsible content and can be collapsed/expanded.
 *
 * Auto-save has been removed; the parent page's top Save button
 * collects payloads from all cards via the ref and dispatches the API
 * calls in a single batch.
 */
const GenericProviderInstanceCard = forwardRef<
  ProviderInstanceCardRef,
  ProviderInstanceCardProps
>(function GenericProviderInstanceCard(
  { providerName, instance, isDraft = false, onDelete, defaultOpen = false },
  ref,
) {
  // Drafts always start open (the user just added them and needs to
  // fill the fields); saved cards default to collapsed unless the
  // parent flagged this card as the one to expand initially (typically
  // the first instance in the list).
  const [open, setOpen] = useState(isDraft || defaultOpen);
  // Drafts start with an empty name - the user types it themselves.
  const [draftName, setDraftName] = useState('');
  // For saved cards: the instance name as shown in the UI. Initialized
  // from the persisted name and updated when the user double-clicks the
  // name to rename it. Persisted via the top Save button.
  const [editedInstanceName, setEditedInstanceName] = useState(
    instance.instance_name,
  );
  const formRef = useRef<DynamicFormRef>(null);
  // Mirror of the per-instance model list - written by ModelsSection
  // via `setModelInfo`, read by the payload builder.
  const modelInfoRef = useRef<IModelInfo[]>([]);

  // Provider-specific config: carries `verifyTransform` / `submitTransform`
  // for providers whose form field names don't map directly onto
  // `api_key` / `base_url` (e.g. OpenDataLoader's nested
  // `opendataloader_apiserver` / `opendataloader_api_key`). When present
  // the transforms take precedence over the generic field mapping inside
  // `useVerifyProvider` and `useInstanceSaveState.buildPayload`.
  const providerConfig = useMemo(
    () => getProviderConfig(providerName),
    [providerName],
  );

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
    }
  }, [providerName, isDraft]);

  // Reset the edited name when the persisted instance changes (e.g.
  // after a successful rename + refetch, or when switching providers).
  useEffect(() => {
    if (!isDraft) {
      setEditedInstanceName(instance.instance_name);
    }
  }, [instance.instance_name, isDraft]);

  // ── Data fetching ────────────────────────────────────────────────
  const { instanceNameSet } = useFetchInstanceNameSet(
    isDraft ? providerName : '',
  );
  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);
  const { baseUrlOptions } = useProviderBaseUrlOptions(providerName);
  const { instanceDetails } = useLazyInstanceDetails(
    providerName,
    instance.instance_name,
    isDraft,
    open,
  );

  // ── Form initial values + fields ────────────────────────────────
  const initialValues = useProviderInitialValues(
    instance,
    instanceDetails,
    isDraft,
    baseUrlOptions,
  );
  const { formFields, formDefaultValues } = useFormFields(
    providerName,
    isDraft,
    initialValues,
    baseUrlOptions,
    hideWhenInstanceExists,
  );
  useFormResetOnDetailsLoad(
    formRef,
    formDefaultValues,
    instanceDetails,
    isDraft,
  );

  // ── Action handlers ─────────────────────────────────────────────
  const handleVerify = useVerifyProvider(
    providerName,
    formRef,
    providerConfig.verifyTransform,
  );
  const handleDelete = useDeleteInstance(
    providerName,
    instance.instance_name,
    isDraft,
    onDelete,
  );

  // ── Save state (payload builder + dirty tracking) ───────────────
  const { getSavePayload, markSaved, markModelsEdited } = useInstanceSaveState({
    formRef,
    providerName,
    instanceName: instance.instance_name,
    editedInstanceName,
    instanceId: instance.id,
    isDraft,
    draftName,
    instanceDetails,
    initialValues,
    modelInfoRef,
    submitTransform: providerConfig.submitTransform,
  });

  // Expose the imperative save API to the parent so the top-of-page
  // Save button can validate, collect payloads, and dispatch calls
  // in a single batch without each card wiring its own auto-save.
  useImperativeHandle(
    ref,
    () => ({
      validate: async () => {
        // Drafts need a non-empty instance name (the DynamicForm does
        // not include the instance_name field, so `trigger()` won't
        // catch it). For both drafts and saved cards, run the form's
        // own validation so errors surface in the UI.
        if (isDraft && !draftName.trim()) return false;
        const isValid = await formRef.current?.trigger();
        return !!isValid;
      },
      getSavePayload,
      markSaved,
    }),
    [isDraft, draftName, getSavePayload, markSaved],
  );

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {isDraft ? (
        <DraftModeCard
          formFields={formFields}
          formDefaultValues={formDefaultValues}
          formRef={formRef}
          handleVerify={handleVerify}
          handleDelete={handleDelete}
          handleInstanceModelsEdited={markModelsEdited}
          providerName={providerName}
          instanceName={instance.instance_name}
          instance={instance}
          modelInfoRef={modelInfoRef}
          draftName={draftName}
          setDraftName={setDraftName}
        />
      ) : (
        <SavedModeCard
          formFields={formFields}
          formDefaultValues={formDefaultValues}
          formRef={formRef}
          handleVerify={handleVerify}
          handleDelete={handleDelete}
          handleInstanceModelsEdited={markModelsEdited}
          providerName={providerName}
          instanceName={instance.instance_name}
          editedInstanceName={editedInstanceName}
          onRename={setEditedInstanceName}
          instance={instance}
          instanceDetailsLoaded={Boolean(instanceDetails)}
          modelInfoRef={modelInfoRef}
          draftName={draftName}
          open={open}
          setOpen={setOpen}
        />
      )}
    </div>
  );
});

/**
 * One inline provider-instance card. The provider name + doc-link arrow
 * live in the parent page's sticky `ProviderHeaderBar`; this card only
 * shows the **instance**-level details (name, fields, verify, models).
 *
 * Two visual modes driven by `isDraft`:
 *  1. **Draft** (`isDraft`): the instance name lives in a dedicated
 *     input section at the top of the body, and the form fields are
 *     always editable (no fieldset lock). The parent drives save
 *     through the imperative ref API.
 *  2. **Saved** (`!isDraft`): the form-field section collapses into a
 *     single collapsible row showing the name as plain text with a
 *     hover-only key/lock icon. The form fields live inside the
 *     collapsible content and can be collapsed/expanded.
 *
 * Auto-save has been removed; the parent page's top Save button
 * collects payloads from all cards via the ref and dispatches the API
 * calls in a single batch.
 */
export const ProviderInstanceCard = forwardRef<
  ProviderInstanceCardRef,
  ProviderInstanceCardProps
>(function ProviderInstanceCard(props, ref) {
  // AWS Bedrock has provider-specific fields (auth_mode, region, AK/SK,
  // role ARN, model name, max_tokens) that don't fit the generic
  // DynamicForm path. Render its own inline card instead.
  //
  // SoMark is similar: its many provider-specific fields (image /
  // formula / table / cs formats + 7 boolean feature toggles) don't
  // fit the generic DynamicForm path. Render its own inline card too.
  //
  // Dispatch BEFORE any hooks so each branch component has a stable
  // hook-call order (Rules of Hooks).
  if (props.providerName === 'Bedrock') {
    return <BedrockInstanceCard {...props} ref={ref} />;
  }
  if (props.providerName === 'SoMark') {
    return <SoMarkInstanceCard {...props} ref={ref} />;
  }
  return <GenericProviderInstanceCard {...props} ref={ref} />;
});

// Re-export the name section for callers that need to embed it
// (e.g. parent pages with custom layouts).
export { InstanceNameSection };

export default ProviderInstanceCard;
