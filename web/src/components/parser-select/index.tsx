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

import {
  SelectWithSearch,
  type SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import {
  parseParserOptionValue,
  ParserOptionKind,
  useParserOptions,
} from '@/hooks/use-parser-options';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

interface IProps {
  // Prefixed value produced by buildParserOptionValue, e.g. "builtin:general".
  value?: string;
  // Called with the resolved kind and the raw backend id on selection.
  onChange: (kind: ParserOptionKind | null, rawId: string) => void;
  placeholder?: string;
  disabled?: boolean;
}

// ParserSelect is the single dropdown that merges pipeline (canvas) and builtin
// parser options. It is a controlled component: the parent owns the form fields
// and translates the kind/id pair into parser_id / pipeline_id / parse_type.
export function ParserSelect({
  value,
  onChange,
  placeholder,
  disabled,
}: IProps) {
  const { t } = useTranslation();
  const { options, loading } = useParserOptions();

  // Trim: some locales still define the marker with surrounding whitespace
  // from when it was appended to the label text.
  const builtInTag = t('knowledgeConfiguration.builtInSuffix').trim();

  // Render the builtin marker as a tag next to the label instead of baking it
  // into the label text; keywords keep the plain label searchable via cmdk.
  const selectOptions = useMemo<SelectWithSearchFlagOptionType[]>(
    () =>
      options.map((option) => {
        if (option.kind !== ParserOptionKind.BuiltIn) {
          return option;
        }
        return {
          ...option,
          label: (
            <>
              {option.label}
              <span className="shrink-0 rounded-md bg-bg-card px-1.5 py-0.5 text-xs text-text-secondary">
                {builtInTag}
              </span>
            </>
          ),
          keywords: [option.label],
        };
      }),
    [options, builtInTag],
  );

  // When the saved value references a parser that no longer exists (e.g. a
  // deleted pipeline) and options have finished loading, render a localized
  // "unavailable" label instead of the raw internal value.
  const renderMissingParser = useCallback(
    () =>
      t('knowledgeConfiguration.parserOptionUnavailable', {
        defaultValue: 'unavailable',
      }),
    [t],
  );

  const handleChange = useCallback(
    (v: string) => {
      const parsed = parseParserOptionValue(v);
      if (!parsed) {
        onChange(null, '');
        return;
      }
      onChange(parsed.kind, parsed.rawId);
    },
    [onChange],
  );

  return (
    <SelectWithSearch
      value={value}
      loading={loading}
      renderMissingValue={renderMissingParser}
      onChange={handleChange}
      placeholder={
        placeholder ?? t('knowledgeConfiguration.parserSelectPlaceholder')
      }
      options={selectOptions}
      disabled={disabled}
    />
  );
}
