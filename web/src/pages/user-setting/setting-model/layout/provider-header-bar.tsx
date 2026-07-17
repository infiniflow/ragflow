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

import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { APIMapUrl } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { ArrowUpRight, Loader2, Save } from 'lucide-react';
import { getProviderConfig } from '../provider-schema/field-config';

interface ProviderHeaderBarProps {
  providerName: string;
  /** Called when the user clicks the batch Save button. */
  onSave?: () => void;
  /** True while the batch save is in flight - disables the button. */
  saving?: boolean;
  /**
   * True when there is at least one dirty card on the page. When false
   * the Save button is disabled (nothing to persist).
   */
  canSave?: boolean;
}

/**
 * Sticky top bar for the right pane that displays the selected provider's
 * icon, name, an API-link arrow, an optional integration-doc link, and a
 * batch Save button. Stays visible while the user scrolls the instance
 * list below.
 */
export function ProviderHeaderBar({
  providerName,
  onSave,
  saving = false,
  canSave = false,
}: ProviderHeaderBarProps) {
  const { t: tSetting } = useTranslate('setting');
  const apiLink = APIMapUrl[providerName as keyof typeof APIMapUrl];
  const providerConfig = getProviderConfig(providerName);
  const docLink = providerConfig.docLink;
  // Resolve doc-link text: explicit `docLinkText` wins, otherwise translate
  // `docLinkI18nKey` with the provider name as the `{{name}}` interpolation.
  const docLinkText =
    providerConfig.docLinkText ??
    (providerConfig.docLinkI18nKey
      ? tSetting(providerConfig.docLinkI18nKey, { name: providerName })
      : null);

  return (
    <div
      className="sticky top-0 z-10 bg-bg-base flex items-center gap-2 px-4 py-3 border-b border-border-button"
      data-testid={`provider-header-${providerName}`}
    >
      <LlmIcon
        name={providerName}
        width={24}
        height={24}
        imgClass="size-6 text-text-primary"
      />
      <span className="font-medium text-text-primary">{providerName}</span>
      {apiLink && (
        <a
          href={apiLink}
          target="_blank"
          rel="noopener noreferrer"
          className="text-text-secondary hover:text-text-primary"
          aria-label={tSetting('docLink')}
        >
          <ArrowUpRight className="size-4" />
        </a>
      )}
      <div className="w-5" />
      {docLink && docLinkText && (
        <a
          href={docLink}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-end self-end gap-1 text-xs text-text-secondary hover:text-text-primary"
        >
          <span>{docLinkText}</span>
        </a>
      )}
      <div className="flex-1" />
      <Button
        type="button"
        size="sm"
        onClick={onSave}
        disabled={saving || !canSave}
        data-testid="provider-save-all"
        className="gap-1.5"
      >
        {saving ? (
          <Loader2 className="size-4 animate-spin" />
        ) : (
          <Save className="size-4" />
        )}
        {tSetting('saveAll')}
      </Button>
    </div>
  );
}

export default ProviderHeaderBar;
