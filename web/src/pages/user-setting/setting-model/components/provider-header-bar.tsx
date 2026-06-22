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
import { APIMapUrl } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { ArrowUpRight } from 'lucide-react';

interface ProviderHeaderBarProps {
  providerName: string;
}

/**
 * Sticky top bar for the right pane that displays the selected provider's
 * icon, name and a doc-link arrow. Stays visible while the user scrolls
 * the instance list below.
 */
export function ProviderHeaderBar({ providerName }: ProviderHeaderBarProps) {
  const { t: tSetting } = useTranslate('setting');
  const docLink = APIMapUrl[providerName as keyof typeof APIMapUrl];

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
      {docLink && (
        <a
          href={docLink}
          target="_blank"
          rel="noopener noreferrer"
          className="text-text-secondary hover:text-text-primary"
          aria-label={tSetting('docLink')}
        >
          <ArrowUpRight className="size-4" />
        </a>
      )}
    </div>
  );
}

export default ProviderHeaderBar;
