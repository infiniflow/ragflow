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
import { useTranslate } from '@/hooks/common-hooks';
import {
  LlmKeys,
  useAddProviderInstance,
  useFetchProviderInstances,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { useQueryClient } from '@tanstack/react-query';
import { Plus } from 'lucide-react';
import { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ProviderHeaderBar } from './components/provider-header-bar';
import { ProviderInstanceCard } from './components/provider-instance-card';
import { Sidebar, SidebarSelection } from './components/sidebar';
import SystemSetting from './components/system-setting';

/**
 * Sidebar-driven model provider settings page.
 *
 * Layout:
 *  - Left: `Sidebar` (Default-models entry, search, provider list).
 *  - Right:
 *      * 'default' selection -> `SystemSetting`.
 *      * provider selection  -> a sticky `ProviderHeaderBar` at the top,
 *        a vertical stack of `ProviderInstanceCard` in the middle, and
 *        a sticky "+ Instance" button at the bottom. Each click of
 *        that button adds a new draft card; multiple drafts can coexist
 *        and be saved / cancelled independently.
 */
const SettingModelV2: FC = () => {
  const { t: tSetting } = useTranslate('setting');
  const [selection, setSelection] = useState<SidebarSelection>('default');
  // Stack of draft-instance identifiers, rendered as `ProviderInstanceCard`
  // entries below the persisted instances. Each draft can be saved or
  // cancelled independently; saving removes the draft from this list and
  // triggers a refetch that surfaces the newly-saved instance above.
  const [draftIds, setDraftIds] = useState<string[]>([]);
  // Monotonic counter so each draft card has a stable, unique React key.
  const draftIdCounterRef = useRef(0);
  // Tracks whether the user explicitly cancelled the auto-shown draft
  // for the current selection. Reset on every selection change.
  const cancelledRef = useRef(false);

  // Always re-fetch when the selection changes. Passing an empty string
  // disables the query.
  const providerQueryName = selection === 'default' ? '' : selection;
  const { data: instances } = useFetchProviderInstances(providerQueryName);
  const queryClient = useQueryClient();

  // Append a new draft id to the visible list.
  const addDraft = useCallback(() => {
    draftIdCounterRef.current += 1;
    const id = `draft-${draftIdCounterRef.current}`;
    setDraftIds((prev) => [...prev, id]);
  }, []);

  // Remove a draft id from the visible list (called on save / cancel).
  const removeDraft = useCallback((id: string) => {
    setDraftIds((prev) => prev.filter((d) => d !== id));
  }, []);

  // When the selection changes, clear the cancelled flag and drop any
  // in-flight drafts so the user starts fresh on the new provider.
  useEffect(() => {
    cancelledRef.current = false;
    setDraftIds([]);
  }, [selection]);

  // If the user switches to a provider with no existing instances and
  // no drafts already on screen, auto-show a "new instance" draft so
  // they can fill it in immediately. If they have explicitly cancelled,
  // do not re-show.
  useEffect(() => {
    if (selection === 'default' || cancelledRef.current) return;
    if ((!instances || instances.length === 0) && draftIds.length === 0) {
      addDraft();
    }
  }, [selection, instances, draftIds, addDraft]);

  const { addProviderInstance } = useAddProviderInstance();

  // Save handler for a draft card. Calls `addProviderInstance` with the
  // values supplied by the draft form (instance_name, api_key, base_url,
  // model_info...). After a successful save, removes the draft from the
  // list and invalidates the instance query so the new card appears in
  // the persisted list automatically.
  const handleDraftSave = useCallback(
    async (id: string, values: Record<string, any>) => {
      const ret = await addProviderInstance({
        llm_factory: selection as string,
        instance_name: values.instance_name,
        api_key: values.api_key,
        base_url: values.base_url ?? values.api_base,
        model_info: values.model_info,
      } as any);
      if (ret?.code === 0) {
        removeDraft(id);
        queryClient.invalidateQueries({
          queryKey: LlmKeys.providerInstances(providerQueryName),
        });
      }
    },
    [
      addProviderInstance,
      selection,
      queryClient,
      providerQueryName,
      removeDraft,
    ],
  );

  // User clicked Cancel on a specific draft — remove it from the list
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

  return (
    <div className="flex w-full h-full border-[0.5px] border-border-button rounded-lg relative overflow-hidden">
      <Spotlight />
      <section className="flex flex-col gap-4 w-[320px] shrink-0 px-5 border-r-[0.5px] border-border-button overflow-auto scrollbar-auto">
        <Sidebar selection={selection} onSelect={setSelection} />
      </section>
      <section className="flex-1 flex flex-col overflow-hidden">
        {selection === 'default' ? (
          <div className="flex-1 overflow-auto scrollbar-auto">
            <SystemSetting />
          </div>
        ) : (
          <>
            {/* Sticky top: provider name + doc-link arrow */}
            <ProviderHeaderBar providerName={selection as string} />

            {/* Scrollable middle: instance cards + optional draft cards */}
            <div className="flex-1 overflow-auto scrollbar-auto p-4 flex flex-col gap-4">
              {instances.length === 0 && draftIds.length === 0 && (
                <div className="text-text-secondary text-sm py-6 text-center">
                  {tSetting('noInstancesConfigured')}
                </div>
              )}
              {instances.map((instance) => (
                <ProviderInstanceCard
                  key={instance.instance_name}
                  providerName={selection as string}
                  instance={instance}
                />
              ))}
              {draftIds.map((id) => (
                <ProviderInstanceCard
                  key={id}
                  providerName={selection as string}
                  instance={draftInstance}
                  isDraft
                  onDelete={() => handleDraftCancel(id)}
                  onNameSaved={() => removeDraft(id)}
                  onSaved={(values) => handleDraftSave(id, values)}
                />
              ))}
              <div className=" bottom-0 z-10 border-border-button py-4">
                <button
                  type="button"
                  className="w-full flex items-center justify-center gap-2 px-3 py-3 rounded-md border border-dashed border-border-button text-text-secondary hover:bg-bg-input hover:text-text-primary transition-colors"
                  onClick={addDraft}
                  data-testid="add-instance-bottom"
                >
                  <Plus className="size-4" />
                  <span className="text-sm">{tSetting('addInstanceText')}</span>
                </button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  );
};

export default SettingModelV2;
