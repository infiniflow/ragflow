import { SearchInput } from '@/components/ui/input';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { cn } from '@/lib/utils';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useFetchKnowledgeGraph } from '@/hooks/use-knowledge-request';
import KnowledgeForceGraph from './knowledge-force-graph';

export enum LeftPanelTab {
  Contents = 'contents',
  Graph = 'graph',
}

type ContentsItem = {
  id: string;
  label: string;
  type?: 'entity' | 'tag' | 'text';
  active?: boolean;
};

const mockContents: ContentsItem[] = [
  { id: '1', label: 'Steven Paul Jobs', type: 'text' },
  { id: '2', label: 'iPhone 17', type: 'entity', active: true },
  { id: '3', label: 'Company', type: 'tag' },
  { id: '4', label: 'iPhone 17', type: 'text' },
  { id: '5', label: 'iPhone 17', type: 'text' },
  { id: '6', label: 'iPhone 17', type: 'text' },
  { id: '7', label: 'iPhone 17', type: 'text' },
  { id: '8', label: 'iPhone 17', type: 'text' },
  { id: '9', label: 'iPhone 17', type: 'text' },
];

type LeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
};

function ContentsList() {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');

  return (
    <div className="size-full flex flex-col">
      <div className="px-3 py-2">
        <SearchInput
          placeholder={t('common.search')}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-3 pb-3">
        <ul className="space-y-1">
          {mockContents.map((item) => (
            <li
              key={item.id}
              className={cn(
                'flex items-center justify-between gap-2 px-3 py-2 rounded-md text-sm cursor-pointer',
                item.active
                  ? 'bg-bg-base text-text-primary'
                  : 'text-text-secondary hover:bg-bg-base hover:text-text-primary',
              )}
            >
              <span className="flex items-center gap-2 min-w-0">
                {item.type === 'tag' && (
                  <span className="text-text-secondary">#</span>
                )}
                <span className="truncate">{item.label}</span>
              </span>
              {item.type === 'entity' && (
                <span className="text-xs text-text-disabled uppercase tracking-wide">
                  {t('knowledgeDetails.entity')}
                </span>
              )}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

export function WikiLeftPanel({ tab, onTabChange }: LeftPanelProps) {
  const { t } = useTranslation();
  const { data } = useFetchKnowledgeGraph();

  return (
    <aside className="size-full flex flex-col">
      <Tabs value={tab} onValueChange={onTabChange} className="p-3">
        <TabsList className="w-full grid grid-cols-2">
          <TabsTrigger value={LeftPanelTab.Contents}>
            {t('knowledgeDetails.contents')}
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph}>
            {t('knowledgeDetails.graph')}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="flex-1 min-h-0 relative">
        {tab === LeftPanelTab.Contents && <ContentsList />}
        {tab === LeftPanelTab.Graph && (
          <KnowledgeForceGraph data={data?.graph} show />
        )}
      </div>
    </aside>
  );
}
