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

import Spotlight from '@/components/spotlight';
import message from '@/components/ui/message';
import { useTranslate } from '@/hooks/common-hooks';
import {
  LLMApiAction,
  LlmKeys,
  useAddProviderInstance,
  useFetchAddedProviders,
  useFetchProviderInstances,
  useUpdateProviderInstance,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { useIsMutating, useQueryClient } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ProviderInstanceCardRef } from './instance-card/interface';
import { ProviderInstanceCard } from './instance-card/provider-instance-card';
import { ProviderHeaderBar } from './layout/provider-header-bar';
import { Sidebar, SidebarSelection } from './layout/sidebar';
import SystemSetting from './layout/system-setting';

/**
 * Sidebar-driven model provider settings page.
 *
 * Layout:
 *  - Left: `Sidebar` (Default-models entry, search, provider list).
 *  - Right:
 *      * 'default' selection -> `SystemSetting`.
 *      * provider selection  -> a sticky `ProviderHeaderBar` at the top
 *        (with a batch Save button), a vertical stack of
 *        `ProviderInstanceCard` in the middle, and a sticky "+ Instance"
 *        button at the bottom. Each click of that button adds a new
 *        draft card; multiple drafts can coexist.
 *
 * Save flow: the top Save button validates every visible card through
 * the imperative ref API; if all are valid it collects each card's
 * payload (skipping non-dirty saved cards) and dispatches one API call
 * per dirty card - `addProviderInstance` for drafts and Bedrock
 * saved cards, `updateProviderInstance` for generic saved cards.
 *
 * Special-case providers (handled inside `ProviderInstanceCard`):
 *  - `Bedrock`: rendered inline via `BedrockInstanceCard`.
 *  - All other providers (including SoMark) use the generic
 *    `GenericProviderInstanceCard` path.
 */
const SettingModelV2: FC = () => {
  const { t: tSetting } = useTranslate('setting');
  const { t: tFlow } = useTranslate('flow');
  const [selection, setSelection] = useState<SidebarSelection>('default');
  // Stack of draft-instance identifiers, rendered as `ProviderInstanceCard`
  // entries below the persisted instances. Each draft can be cancelled
  // independently; saving is driven by the top Save button.
  const [draftIds, setDraftIds] = useState<string[]>([]);

  const [saving, setSaving] = useState(false);
  const modelMutationCount = useIsMutating({
    predicate: (mutation) => {
      const action = mutation.options.mutationKey?.[0];
      return (
        action === LLMApiAction.AddInstanceModel ||
        action === LLMApiAction.DeleteInstanceModels ||
        action === LLMApiAction.PatchInstanceModel ||
        action === LLMApiAction.UpdateProviderInstance
      );
    },
  });
  const modelMutationPending = modelMutationCount > 0;

  // Tracks the instance name that was just persisted by the top Save
  // button. The corresponding saved card mounts expanded so the user
  // can immediately see (and edit) what was just saved. Reset on every
  // selection change so it does not bleed across providers.
  const [newlySavedInstanceName, setNewlySavedInstanceName] = useState<
    string | null
  >(null);

  // Monotonic counter so each draft card has a stable, unique React key.
  const draftIdCounterRef = useRef(0);
  // Tracks whether the user explicitly cancelled the auto-shown draft
  // for the current selection. Reset on every selection change.
  const cancelledRef = useRef(false);

  // Imperative refs to every visible card, keyed by the card's React key
  // (instance name for saved cards, draft id for drafts). Used by the
  // top Save button to validate + collect payloads in a single batch.
  const cardRefs = useRef<Map<string, ProviderInstanceCardRef | null>>(
    new Map(),
  );
  const setCardRef = useCallback(
    (id: string) => (ref: ProviderInstanceCardRef | null) => {
      if (ref) {
        cardRefs.current.set(id, ref);
      } else {
        cardRefs.current.delete(id);
      }
    },
    [],
  );

  const { data: addedProviders } = useFetchAddedProviders();
  const providerQueryName = useMemo(() => {
    if (selection === 'default') return '';
    return addedProviders.some((p) => p.name === selection && p.has_instance)
      ? selection
      : '';
  }, [selection, addedProviders]);
  const { data: instances, loading: instancesLoading } =
    useFetchProviderInstances(providerQueryName);
  const queryClient = useQueryClient();

  // Append a new draft id to the visible list.
  const addDraft = useCallback(() => {
    draftIdCounterRef.current += 1;
    const id = `draft-${draftIdCounterRef.current}`;
    setDraftIds((prev) => [...prev, id]);
  }, []);

  // Remove a draft id from the visible list (called on cancel).
  const removeDraft = useCallback((id: string) => {
    setDraftIds((prev) => prev.filter((d) => d !== id));
  }, []);

  // When the selection changes, clear the cancelled flag, drop any
  // in-flight drafts, and reset the ref registry so stale refs from
  // the previous provider don't leak into the next save batch.
  useEffect(() => {
    cancelledRef.current = false;
    setDraftIds([]);
    cardRefs.current.clear();
    setNewlySavedInstanceName(null);
  }, [selection]);

  // If the user switches to a provider with no existing instances and
  // no drafts already on screen, auto-show a "new instance" draft so
  // they can fill it in immediately. If they have explicitly cancelled,
  // do not re-show.
  //
  // Wait for the provider-instances query to settle before deciding:
  // `initialData: []` on the hook means `instances` is an empty array
  // from the first render, so a naive length check would auto-spawn a
  // draft during the brief loading window even when the provider
  // already has saved instances, only to remove it once the query
  // resolves. Gating on `!instancesLoading` skips that flicker and
  // keeps the UI clean for providers that do have instances.
  useEffect(() => {
    if (selection === 'default' || cancelledRef.current) return;
    if (instancesLoading) return;
    if (instances.length === 0 && draftIds.length === 0) {
      addDraft();
    }
  }, [selection, instances, instancesLoading, draftIds, addDraft]);

  const { addProviderInstance } = useAddProviderInstance();
  const { updateProviderInstance } = useUpdateProviderInstance();

  // Batch save handler, wired to the top Save button.
  //
  // Flow:
  //   1. Collect every card ref.
  //   2. Ask each for its save payload. Drafts always return one
  //      (provided the name is non-empty); saved cards return `null`
  //      when not dirty so we skip the redundant API call.
  //   3. If any card is invalid (or a draft has no name), abort the
  //      whole batch - errors are surfaced in the form UI by `trigger()`.
  //   4. Dispatch one API call per dirty card, in order. Drafts and
  //      Bedrock saved cards go through `addProviderInstance`
  //      (Bedrock saved cards carry an `id` so the backend
  //      updates instead of creating); generic and SoMark saved cards
  //      go through `updateProviderInstance`.
  //   5. On each success: remove that persisted draft or mark the saved
  //      card's baseline so a later failure cannot leave acknowledged
  //      state looking editable. Finally refresh the instance list.
  const handleSaveAll = useCallback(async () => {
    if (modelMutationPending) return;
    const refs = Array.from(cardRefs.current.entries()).flatMap(
      ([cardId, ref]) => (ref ? [{ cardId, ref }] : []),
    );
    if (refs.length === 0) return;

    const participating = refs
      .map(({ cardId, ref }) => ({
        cardId,
        ref,
        payload: ref.getSavePayload(),
      }))
      .filter(
        (entry) => entry.payload !== null || draftIds.includes(entry.cardId),
      );
    if (participating.length === 0) return;

    // Freeze the cards before the first async boundary. Payloads above were
    // captured synchronously, so validation cannot race with later edits.
    setSaving(true);
    // Pin the auto-show guard so a draft isn't re-spawned while the
    // providerInstances refetch is in flight (during that window both
    // `instances` and `draftIds` are empty and would otherwise re-trigger
    // `addDraft()`).
    cancelledRef.current = true;
    try {
      const validations = await Promise.all(
        participating.map(async (entry) => ({
          ...entry,
          valid: await entry.ref.validate(),
        })),
      );
      if (validations.some((validation) => !validation.valid)) {
        return;
      }

      // 3. Dispatch one API call per dirty card. Sequential so any
      //    backend error stops the batch and the user can retry the
      //    remaining cards after fixing the issue.
      for (const { cardId, ref, payload } of validations) {
        if (!payload) continue;
        if (payload.apiKind === 'add') {
          const ret = await addProviderInstance(payload.payload as any);
          if (ret?.code !== 0) {
            // No write was acknowledged; preserve this and later drafts.
            return;
          }
          if (ret.skippedExisting) {
            message.error(
              `${payload.instanceName}: ${tFlow('nameRepeatedMsg')}`,
            );
            return;
          }
          // Remember the just-saved name so the persisted card mounts
          // expanded once it surfaces via the invalidated instances
          // query below.
          if (payload.isDraft) {
            setDraftIds((previous) => previous.filter((id) => id !== cardId));
            cardRefs.current.delete(cardId);
            setNewlySavedInstanceName(payload.instanceName);
          }
        } else {
          const ret = await updateProviderInstance(payload.payload as any);
          if (ret?.code !== 0) {
            return;
          }
          // Saved card: update its dirty baseline so the next save
          // short-circuits.
          ref.markSaved(payload.payload);
        }
      }
      // 4. Refresh the instance query so newly-saved cards appear.
      // Drafts were removed individually after their own acknowledged
      // response, so empty or failed drafts remain available to fix.
      queryClient.invalidateQueries({
        queryKey: LlmKeys.providerInstances(providerQueryName),
      });
    } finally {
      setSaving(false);
    }
  }, [
    addProviderInstance,
    updateProviderInstance,
    queryClient,
    providerQueryName,
    draftIds,
    modelMutationPending,
    tFlow,
  ]);

  // Whether the Save button should be enabled. We avoid an O(n) ref
  // scan on every render by treating "has any draft OR any saved
  // instance" as a conservative proxy - if there is nothing on screen
  // there is nothing to save, and if there is something the user can
  // always attempt a save (dirty saved cards short-circuit inside
  // `getSavePayload`). The button is disabled while a save is in flight.
  const canSave =
    !saving &&
    !modelMutationPending &&
    (draftIds.length > 0 || instances.length > 0);

  // User clicked Cancel on a specific draft - remove it from the list
  // and stop the auto-show effect from re-opening it for the current
  // empty-instance selection.
  const handleDraftCancel = useCallback(
    (id: string) => {
      cancelledRef.current = true;
      removeDraft(id);
    },
    [removeDraft],
  );

  const draftInstance: IProviderInstance = useMemo(
    () => ({ instance_name: '' }) as IProviderInstance,
    [],
  );

  const handleSelectionChange = useCallback(
    (nextSelection: SidebarSelection) => {
      if (!saving) setSelection(nextSelection);
    },
    [saving],
  );

  return (
    <div className="flex w-full h-full border-[0.5px] border-border-button rounded-lg relative overflow-hidden">
      <Spotlight />
      <section className="flex flex-col gap-4 w-[320px] shrink-0 px-5 border-r-[0.5px] border-border-button overflow-auto scrollbar-auto">
        <Sidebar selection={selection} onSelect={handleSelectionChange} />
      </section>
      <section className="flex-1 flex flex-col overflow-hidden">
        {selection === 'default' ? (
          <div className="flex-1 overflow-auto scrollbar-auto">
            <SystemSetting />
          </div>
        ) : (
          <>
            {/* Sticky top: provider name + doc-link arrow + batch Save */}
            <ProviderHeaderBar
              providerName={selection as string}
              onSave={handleSaveAll}
              saving={saving}
              canSave={canSave}
            />

            {/* Scrollable middle: instance cards + optional draft cards */}
            <fieldset
              disabled={saving}
              className="flex-1 min-w-0 border-0 overflow-auto scrollbar-auto p-4 flex flex-col gap-4"
            >
              {instances.length === 0 && draftIds.length === 0 && (
                <div className="text-text-secondary text-sm py-6 text-center">
                  {tSetting('noInstancesConfigured')}
                </div>
              )}
              {instances.map((instance, index) => (
                <ProviderInstanceCard
                  key={instance.instance_name}
                  ref={setCardRef(instance.instance_name)}
                  providerName={selection as string}
                  instance={instance}
                  defaultOpen={
                    index === 0 ||
                    instance.instance_name === newlySavedInstanceName
                  }
                />
              ))}
              {draftIds.map((id) => (
                <ProviderInstanceCard
                  key={id}
                  ref={setCardRef(id)}
                  providerName={selection as string}
                  instance={draftInstance}
                  isDraft
                  onDelete={() => handleDraftCancel(id)}
                />
              ))}
              <div className="z-10 border-border-button py-4">
                <button
                  type="button"
                  className="w-full flex items-center justify-center gap-2 px-3 py-1 rounded-md border border-dashed border-border-button text-text-secondary hover:bg-bg-input hover:text-text-primary transition-colors"
                  onClick={addDraft}
                  data-testid="add-instance-bottom"
                >
                  <Plus className="size-4" />
                  <span className="text-sm">{tSetting('addInstanceText')}</span>
                </button>
              </div>
            </fieldset>
          </>
        )}
      </section>
    </div>
  );
};

export default SettingModelV2;
