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

import { buttonVariants } from '@/components/ui/button';
import { APIMapUrl, LLMFactory } from '@/constants/llm';
import { KeyRound } from 'lucide-react';
import { FC } from 'react';
import { useTranslation } from 'react-i18next';

/** Where an AI/ML API account creates and copies its API keys — the same
 *  URL the provider header and the model list link to. */
export const AIMLAPI_KEYS_URL = APIMapUrl[LLMFactory.AIMLAPI];

/**
 * "Get API key" control for the AIMLAPI provider dialog. Opens the AI/ML API
 * dashboard in a new tab, where the user creates a key and pastes it into the
 * API Key field above.
 */
export const AimlapiGetKeyButton: FC = () => {
  const { t } = useTranslation('translation', { keyPrefix: 'setting' });

  return (
    <div className="flex flex-col gap-1.5 mb-4">
      <div className="flex items-center gap-2">
        {/* A link styled as a button. Not <Button asChild>: Button wraps its
            children in a fragment, so Slot would receive the fragment rather
            than the <a> and the styles would be lost. */}
        <a
          href={AIMLAPI_KEYS_URL}
          target="_blank"
          rel="noopener noreferrer"
          className={buttonVariants({ variant: 'secondary', size: 'sm' })}
        >
          <KeyRound size={14} />
          {t('aimlapiGetKey')}
        </a>
      </div>
    </div>
  );
};

export default AimlapiGetKeyButton;
