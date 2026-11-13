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

import JsonEditor from '@/components/json-edit';
import { useTranslation } from 'react-i18next';

interface JsonPreviewProps {
  value: Record<string, unknown>;
}

export function JsonPreview({ value }: JsonPreviewProps) {
  const { t } = useTranslation();

  return (
    <section
      className="min-h-0 min-w-0 flex-1 p-5"
      aria-label={t('knowledgeCompilation.jsonPreview')}
    >
      <JsonEditor
        value={value}
        height="100%"
        options={{ mode: 'tree', modes: ['tree', 'code'] }}
        defaultExpanded
      />
    </section>
  );
}
