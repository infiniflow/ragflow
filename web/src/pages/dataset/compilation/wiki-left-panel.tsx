import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { IArtifact } from '@/interfaces/database/dataset';
import { cn } from '@/lib/utils';
import { useTranslation } from 'react-i18next';

import {
  useFetchArtifactGraph,
  useFetchArtifactList,
} from '@/hooks/use-knowledge-request';
import ArtifactForceGraph from './artifact-force-graph';

export enum LeftPanelTab {
  Contents = 'contents',
  Graph = 'graph',
}

type LeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
};

type ContentListItemProps = {
  item: IArtifact;
  isSelected: boolean;
  onSelect: (artifact: IArtifact) => void;
};

function ContentListItem({ item, isSelected, onSelect }: ContentListItemProps) {
  const handleClick = () => {
    onSelect(item);
  };

  return (
    <li
      key={item.slug}
      onClick={handleClick}
      className={cn(
        'flex items-center justify-between gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
        'text-text-secondary hover:bg-bg-base hover:text-text-primary',
        isSelected && 'bg-bg-card text-text-primary',
      )}
    >
      <div>
        <span className="truncate">{item.title}</span>
        {item.page_type && (
          <span className="text-text-secondary ml-2">{item.page_type}</span>
        )}
      </div>
    </li>
  );
}

type ContentListProps = {
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
};

function ContentList({ selectedArtifact, onSelectArtifact }: ContentListProps) {
  const { t } = useTranslation();
  const {
    artifacts,
    loading,
    searchString,
    handleSearchChange,
    handleScroll,
    hasMore,
  } = useFetchArtifactList();

  return (
    <div className="size-full flex flex-col">
      <div className="px-3 py-2">
        <SearchInput
          placeholder={t('common.search')}
          value={searchString}
          onChange={handleSearchChange}
        />
      </div>

      <div
        className="flex-1 min-h-0 overflow-y-auto px-3 pb-3"
        onScroll={handleScroll}
      >
        <ul className="space-y-1">
          {artifacts.map((item) => (
            <ContentListItem
              key={item.slug}
              item={item}
              isSelected={selectedArtifact?.slug === item.slug}
              onSelect={onSelectArtifact}
            />
          ))}
        </ul>
        {loading && (
          <div className="py-4 flex justify-center">
            <Spin size="small" />
          </div>
        )}
        {!loading && !hasMore && artifacts.length > 0 && (
          <div className="py-2 text-center text-sm text-text-secondary">
            {t('knowledgeList.noMoreData')}
          </div>
        )}
      </div>
    </div>
  );
}

export function WikiLeftPanel({
  tab,
  onTabChange,
  selectedArtifact,
  onSelectArtifact,
}: LeftPanelProps) {
  const { t } = useTranslation();
  const { data } = useFetchArtifactGraph();

  return (
    <aside className="size-full flex flex-col">
      <Tabs value={tab} onValueChange={onTabChange} className="p-3">
        <TabsList className="grid grid-cols-2 w-80">
          <TabsTrigger value={LeftPanelTab.Contents}>
            {t('knowledgeDetails.contents')}
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph}>
            {t('knowledgeDetails.graph')}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="flex-1 min-h-0 relative">
        {tab === LeftPanelTab.Contents && (
          <ContentList
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
          />
        )}
        {tab === LeftPanelTab.Graph && (
          <ArtifactForceGraph
            data={data}
            show
            mapNodeToValue={(node) => ({
              slug: node.slug,
              title: node.name,
              page_type: node.type,
            })}
            onNodeClick={onSelectArtifact}
          />
        )}
      </div>
    </aside>
  );
}
