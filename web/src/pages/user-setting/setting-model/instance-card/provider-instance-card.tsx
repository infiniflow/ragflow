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
import { useEffect, useRef, useState } from 'react';
import { useFetchInstanceNameSet, useHideWhenInstanceExists } from '../hooks';
import { BedrockInstanceCard } from './bedrock-instance-card';
import { DraftModeCard } from './components/draft-mode-card';
import { InstanceNameSection } from './components/instance-name-section';
import { SavedModeCard } from './components/saved-mode-card';
import {
  useDeleteInstance,
  useDraftAutoSave,
  useFormFields,
  useFormResetOnDetailsLoad,
  useLazyInstanceDetails,
  useProviderBaseUrlOptions,
  useProviderInitialValues,
  useSaveInstanceName,
  useSavedAutoSave,
  useVerifyProvider,
} from './hooks';
import { ProviderInstanceCardProps } from './interface';
import { SoMarkInstanceCard } from './somark-instance-card';

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
  // SoMark is similar: its many provider-specific fields (image /
  // formula / table / cs formats + 7 boolean feature toggles) don't
  // fit the generic DynamicForm path. Render its own inline card too.
  //
  // Dispatch BEFORE any hooks so each branch component has a stable
  // hook-call order (Rules of Hooks).
  if (props.providerName === 'Bedrock') {
    return <BedrockInstanceCard {...props} />;
  }
  if (props.providerName === 'SoMark') {
    return <SoMarkInstanceCard {...props} />;
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
  // Mirror of the per-instance model list — written by ModelsSection
  // via `setModelInfo`, read by the auto-save payload assembler.
  const modelInfoRef = useRef<IModelInfo[]>([]);

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
      setNameSaved(false);
    } else {
      setNameSaved(true);
    }
  }, [providerName, isDraft]);

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
  const handleVerify = useVerifyProvider(providerName, formRef);
  const handleSaveName = useSaveInstanceName(
    providerName,
    draftName,
    onNameSaved,
  );
  const handleDelete = useDeleteInstance(
    providerName,
    instance.instance_name,
    isDraft,
    onDelete,
  );

  // ── Auto-save wiring ─────────────────────────────────────────────
  // Draft: 200ms-debounced watch effect, gated on the instance name
  // being entered and saved.
  useDraftAutoSave(
    formRef,
    isDraft,
    nameSaved,
    draftName,
    isDraft ? onSaved : undefined,
    modelInfoRef,
  );

  // Saved: blur-driven + dropdown value-change auto-save via PUT.
  const { handleFieldsBlur, blurSuppressRef, markModelsEdited } =
    useSavedAutoSave({
      formRef,
      formFields,
      providerName,
      instanceName: instance.instance_name,
      instanceId: instance.id,
      isDraft,
      instanceDetails,
      initialValues,
      modelInfoRef,
    });

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {nameSaved ? (
        <SavedModeCard
          formFields={formFields}
          formDefaultValues={formDefaultValues}
          formRef={formRef}
          handleFieldsBlur={handleFieldsBlur}
          handleVerify={handleVerify}
          handleDelete={handleDelete}
          handleInstanceModelsEdited={markModelsEdited}
          providerName={providerName}
          instanceName={instance.instance_name}
          instance={instance}
          instanceDetailsLoaded={Boolean(instanceDetails)}
          modelInfoRef={modelInfoRef}
          blurSuppressRef={blurSuppressRef}
          draftName={draftName}
          open={open}
          setOpen={setOpen}
        />
      ) : (
        <DraftModeCard
          formFields={formFields}
          formDefaultValues={formDefaultValues}
          formRef={formRef}
          handleVerify={handleVerify}
          handleDelete={handleDelete}
          handleSaveName={handleSaveName}
          handleInstanceModelsEdited={markModelsEdited}
          providerName={providerName}
          instanceName={instance.instance_name}
          instance={instance}
          modelInfoRef={modelInfoRef}
          draftName={draftName}
          setDraftName={setDraftName}
        />
      )}
    </div>
  );
}

// Re-export the name section for callers that need to embed it
// (e.g. parent pages with custom layouts).
export { InstanceNameSection };

export default ProviderInstanceCard;
