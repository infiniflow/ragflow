// src/components/AvailableModels.tsx
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { APIMapUrl } from '@/constants/llm';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmList } from '@/hooks/use-llm-request';
import { ArrowUpRight, Plus } from 'lucide-react';
import { FC, useMemo, useState } from 'react';
export const mapModelKey = {
  IMAGE2TEXT: 'VLM',
  'TEXT EMBEDDING': 'Embedding',
  SPEECH2TEXT: 'ASR',
  'TEXT RE-RANK': 'Rerank',
};
const orderMap: Record<TagType, number> = {
  LLM: 1,
  'TEXT EMBEDDING': 2,
  'TEXT RE-RANK': 3,
  TTS: 4,
  SPEECH2TEXT: 5,
  IMAGE2TEXT: 6,
  MODERATION: 7,
};
type TagType =
  | 'LLM'
  | 'TEXT EMBEDDING'
  | 'TEXT RE-RANK'
  | 'TTS'
  | 'SPEECH2TEXT'
  | 'IMAGE2TEXT'
  | 'MODERATION';

const sortTags = (tags: string) => {
  return tags
    .split(',')
    .map((tag) => tag.trim())
    .sort(
      (a, b) =>
        (orderMap[a as TagType] || 999) - (orderMap[b as TagType] || 999),
    );
};

export const AvailableModels: FC<{
  handleAddModel: (factory: string) => void;
}> = ({ handleAddModel }) => {
  const { t } = useTranslate('setting');
  const { factoryList } = useSelectLlmList();

  const [searchTerm, setSearchTerm] = useState('');
  const [selectedTag, setSelectedTag] = useState<string | null>(null);

  const filteredModels = useMemo(() => {
    const models = factoryList.filter((model) => {
      const matchesSearch = model.name
        .toLowerCase()
        .includes(searchTerm.toLowerCase());
      const matchesTag =
        selectedTag === null ||
        model.tags.split(',').some((tag) => tag.trim() === selectedTag);
      return matchesSearch && matchesTag;
    });
    return models;
  }, [factoryList, searchTerm, selectedTag]);

  const allTags = useMemo(() => {
    const tagsSet = new Set<string>();
    factoryList.forEach((model) => {
      model.tags.split(',').forEach((tag) => tagsSet.add(tag.trim()));
    });
    return Array.from(tagsSet).sort(
      (a, b) =>
        (orderMap[a as TagType] || 999) - (orderMap[b as TagType] || 999),
    );
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
        <h3 className="text-text-primary text-base">{t('availableModels')}</h3>
        {/* Search Bar */}
        <div>
          {/* <div className="relative"> */}
          <SearchInput
            data-testid="model-providers-search"
            type="text"
            placeholder={t('search')}
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
            </Button>
          ))}
        </div>
      </header>

      {/* Models List */}
      <div className="p-4 pt-0 flex flex-col gap-4 overflow-auto h-full scrollbar-auto">
        {filteredModels.map((model) => (
          <div
            key={model.name}
            data-testid="available-model-card"
            data-provider={model.name}
            className="group border border-border-button rounded-lg p-3 hover:bg-bg-input transition-colors"
            onClick={() => handleAddModel(model.name)}
          >
            <div className="flex items-center space-x-3 mb-3">
              <LlmIcon name={model.name} imgClass="h-8 w-8 text-text-primary" />
              <div className="flex flex-1 gap-1.5 items-center">
                <div className="font-normal text-base truncate">
                  {model.name}
                </div>
                {!!APIMapUrl[model.name as keyof typeof APIMapUrl] && (
                  <Button
                    asLink
                    variant="ghost"
                    size="icon-xs"
                    className="size-4"
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
              </div>

              <Button
                size="xs"
                className="px-2 opacity-0 transition-all group-hover:opacity-100 group-focus-within:opacity-100"
              >
                <Plus size={12} />
                {t('addTheModel')}
              </Button>
            </div>

            <div className="flex flex-wrap gap-1">
              {sortTags(model.tags).map((tag, index) => (
                <span
                  key={index}
                  className="px-1 flex items-center h-5 text-xs bg-bg-card text-text-secondary rounded-md"
                >
                  {/* {tag} */}
                  {mapModelKey[tag.trim() as keyof typeof mapModelKey] ||
                    tag.trim()}
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>
    </aside>
  );
};
