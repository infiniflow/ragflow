// src/components/AvailableModels.tsx
import { LlmIcon } from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { useSelectLlmList } from '@/hooks/llm-hooks';
import { Plus, Search } from 'lucide-react';
import { FC, useMemo, useState } from 'react';

type TagType =
  | 'LLM'
  | 'TEXT EMBEDDING'
  | 'TEXT RE-RANK'
  | 'TTS'
  | 'SPEECH2TEXT'
  | 'IMAGE2TEXT'
  | 'MODERATION';

const sortTags = (tags: string) => {
  const orderMap: Record<TagType, number> = {
    LLM: 1,
    'TEXT EMBEDDING': 2,
    'TEXT RE-RANK': 3,
    TTS: 4,
    SPEECH2TEXT: 5,
    IMAGE2TEXT: 6,
    MODERATION: 7,
  };

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
    return Array.from(tagsSet).sort();
  }, [factoryList]);

  const handleTagClick = (tag: string) => {
    setSelectedTag(selectedTag === tag ? null : tag);
  };

  return (
    <div className=" text-text-primary h-full p-4">
      <div className="text-text-primary text-base mb-4">
        {t('availableModels')}
      </div>
      {/* Search Bar */}
      <div className="mb-6">
        <div className="relative">
          <Input
            type="text"
            placeholder={t('search')}
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            className="w-full px-4 py-2 pl-10 bg-bg-input border border-border-default rounded-lg focus:outline-none focus:ring-1 focus:ring-border-button transition-colors"
          />
          <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-5 h-5 text-text-secondary" />
        </div>
      </div>

      {/* Tags Filter */}
      <div className="flex flex-wrap gap-2 mb-6">
        <Button
          variant={'secondary'}
          onClick={() => setSelectedTag(null)}
          className={`px-1 py-1 text-xs rounded-sm bg-bg-card h-5 transition-colors ${
            selectedTag === null
              ? ' text-bg-base bg-text-primary '
              : 'text-text-secondary bg-bg-card border-none'
          }`}
        >
          All
        </Button>
        {allTags.map((tag) => (
          <Button
            variant={'secondary'}
            key={tag}
            onClick={() => handleTagClick(tag)}
            className={`px-1 py-1 text-xs rounded-sm bg-bg-card h-5 transition-colors ${
              selectedTag === tag
                ? ' text-bg-base bg-text-primary '
                : 'text-text-secondary  border-none bg-bg-card'
            }`}
          >
            {tag}
          </Button>
        ))}
      </div>

      {/* Models List */}
      <div className="flex flex-col gap-4 overflow-auto h-[calc(100vh-300px)] scrollbar-auto">
        {filteredModels.map((model) => (
          <div
            key={model.name}
            className=" border border-border-button rounded-lg p-3 hover:bg-bg-input transition-colors group"
            onClick={() => handleAddModel(model.name)}
          >
            <div className="flex items-center space-x-3 mb-3">
              <LlmIcon name={model.name} imgClass="h-8 w-8 text-text-primary" />
              <div className="flex-1">
                <h3 className="font-medium truncate">{model.name}</h3>
              </div>
              <Button className=" px-2 items-center gap-0 text-xs h-6  rounded-md transition-colors hidden group-hover:flex">
                <Plus size={12} />
                {t('addTheModel')}
              </Button>
            </div>

            <div className="flex flex-wrap gap-1 mb-3">
              {sortTags(model.tags).map((tag, index) => (
                <span
                  key={index}
                  className="px-1 flex items-center h-5 text-xs bg-bg-card text-text-secondary rounded-md"
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};
