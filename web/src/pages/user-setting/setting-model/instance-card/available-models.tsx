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

// src/components/AvailableModels.tsx
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { APIMapUrl, LLMFactory } from '@/constants/llm';
import { useFetchAvailableProviders } from '@/hooks/use-llm-request';
import { sortLLmFactoryListBySpecifiedOrder } from '@/utils/common-util';
import { ArrowUpRight, Plus, Star } from 'lucide-react';
import { FC, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export const mapModelKey = {
  image2text: 'VLM',
  speech2text: 'ASR',
  chat: 'LLM',
  vision: 'VLM',
  embedding: 'Embedding',
  asr: 'ASR',
  rerank: 'Rerank',
  tts: 'TTS',
  ocr: 'OCR',
};

const orderMap: Record<ModelType, number> = {
  chat: 1,
  embedding: 2,
  rerank: 3,
  tts: 4,
  asr: 5,
  speech2text: 5,
  image2text: 6,
  vision: 6,
  ocr: 7,
};

type ModelType =
  | 'chat'
  | 'embedding'
  | 'rerank'
  | 'tts'
  | 'asr'
  | 'speech2text'
  | 'image2text'
  | 'vision'
  | 'ocr';

export const sortModelTypes = (modelTypes: string[]) => {
  return [...modelTypes].sort(
    (a, b) =>
      (orderMap[a as ModelType] || 999) - (orderMap[b as ModelType] || 999),
  );
};

export const AvailableModels: FC<{
  handleAddModel: (factory: string) => void;
}> = ({ handleAddModel }) => {
  const { t } = useTranslation();
  const { data: factoryList } = useFetchAvailableProviders();

  const [searchTerm, setSearchTerm] = useState('');
  const [selectedTag, setSelectedTag] = useState<string | null>(null);

  const searchedModels = useMemo(() => {
    return factoryList.filter((model) =>
      model.name.toLowerCase().includes(searchTerm.toLowerCase()),
    );
  }, [factoryList, searchTerm]);

  const filteredModels = useMemo(() => {
    const filtered =
      selectedTag === null
        ? searchedModels
        : searchedModels.filter((model) =>
            model.model_types.some((type) => type === selectedTag),
          );
    // AIMLAPI first (then RAGFlow's curated order) in the available list
    return sortLLmFactoryListBySpecifiedOrder(
      filtered as any,
    ) as unknown as typeof filtered;
  }, [searchedModels, selectedTag]);

  // Number of providers matching each tag, respecting the current search term so
  // the badge always reflects how many cards are shown when the tag is selected.
  const tagCounts = useMemo(() => {
    return searchedModels.reduce<Record<string, number>>((acc, model) => {
      // Count each provider once per model type, even if listed more than once.
      new Set(model.model_types).forEach((type) => {
        acc[type] = (acc[type] || 0) + 1;
      });
      return acc;
    }, {});
  }, [searchedModels]);

  const allTags = useMemo(() => {
    const tagsSet = new Set<string>();
    factoryList.forEach((model) => {
      model.model_types.forEach((type) => tagsSet.add(type));
    });
    const res = sortModelTypes(Array.from(tagsSet));
    return res;
  }, [factoryList]);

  const handleTagClick = (tag: string) => {
    setSelectedTag(selectedTag === tag ? null : tag);
  };

  return (
    <aside
      className="text-text-primary h-full flex flex-col"
      data-testid="available-models-section"
    >
      <header className="p-4 space-y-3">
        <h3 className="text-text-primary text-base">
          {t('setting.availableModels')}
        </h3>
        {/* Search Bar */}
        <div>
          {/* <div className="relative"> */}
          <SearchInput
            data-testid="model-providers-search"
            type="text"
            placeholder={t('setting.search')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full px-4 py-2 pl-10 bg-bg-input border border-border-default rounded-lg focus:outline-none focus:ring-1 focus:ring-border-button transition-colors"
          />
          {/* <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-text-secondary" /> */}
          {/* </div> */}
        </div>

        {/* Tags Filter */}
        <div className="flex flex-wrap gap-2">
          <Button
            variant={selectedTag === null ? 'default' : 'secondary'}
            size="auto"
            className="leading-none px-1 py-0.5 rounded-sm text-xs"
            onClick={() => setSelectedTag(null)}
          >
            All
            <span className="ml-1 tabular-nums opacity-60">
              {searchedModels.length}
            </span>
          </Button>

          {allTags.map((tag) => (
            <Button
              variant={selectedTag === tag ? 'default' : 'secondary'}
              size="auto"
              className="leading-none px-1 py-0.5 rounded-sm text-xs"
              key={tag}
              onClick={() => handleTagClick(tag)}
            >
              {mapModelKey[tag.trim() as keyof typeof mapModelKey] ||
                tag.trim()}
              <span className="ml-1 tabular-nums opacity-60">
                {tagCounts[tag] ?? 0}
              </span>
            </Button>
          ))}
        </div>
      </header>

      {/* Models List */}
      <div className="@container p-4 pt-0 flex flex-col gap-4 overflow-auto h-full scrollbar-auto">
        {filteredModels.map((model) => (
          <div
            key={model.name}
            data-testid="available-model-card"
            data-provider={model.name}
            className="group border border-border-button rounded-lg p-3 hover:bg-bg-input transition-colors"
            onClick={() => handleAddModel(model.name)}
          >
            <div className="flex items-center gap-3 mb-3">
              <LlmIcon
                name={model.name}
                imgClass="h-8 w-8 shrink-0 text-text-primary"
              />
              <div className="flex flex-1 min-w-0 gap-1.5 items-center">
                <div className="font-normal text-base truncate">
                  {model.name}
                </div>
                {!!APIMapUrl[model.name as keyof typeof APIMapUrl] && (
                  <Button
                    asLink
                    variant="ghost"
                    size="icon-xs"
                    className="size-4 shrink-0"
                    to={APIMapUrl[model.name as keyof typeof APIMapUrl]}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e: React.MouseEvent<HTMLButtonElement>) =>
                      e.stopPropagation()
                    }
                  >
                    <ArrowUpRight size={16} />
                  </Button>
                )}
                {model.name === LLMFactory.AIMLAPI && (
                  // Recommended badge: the star always shows; the label appears
                  // only when the card (container) is wide enough to fit it
                  // alongside the Add button — otherwise it collapses to just
                  // the star.
                  <span
                    className="shrink-0 inline-flex items-center gap-1 text-state-success"
                    title={t('setting.recommended')}
                    aria-label={t('setting.recommended')}
                  >
                    <Star size={14} className="shrink-0 fill-current" />
                    <span className="hidden @[20rem]:inline text-xs font-medium whitespace-nowrap">
                      {t('setting.recommended')}
                    </span>
                  </span>
                )}
              </div>

              <Button
                size="xs"
                className="shrink-0 px-2 opacity-0 transition-all group-hover:opacity-100 group-focus-within:opacity-100"
              >
                <Plus size={12} />
                {t('setting.addTheModel')}
              </Button>
            </div>

            <div className="flex flex-wrap gap-1">
              {sortModelTypes(model.model_types).map((type, index) => (
                <span
                  key={index}
                  className="px-1 flex items-center h-5 text-xs bg-bg-card text-text-secondary rounded-md"
                >
                  {mapModelKey[type as keyof typeof mapModelKey] || type}
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
};
