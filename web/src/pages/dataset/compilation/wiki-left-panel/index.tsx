import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { IArtifact } from '@/interfaces/database/dataset';
import { useTranslation } from 'react-i18next';

import { LeftPanelTab } from '../constants';
import { WikiGraphPanel } from './wiki-graph-panel';
import { WikiNavBar } from './wiki-nav-bar';

type WikiLeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
  onClearArtifact: () => void;
};

export function WikiLeftPanel({
  tab,
  onTabChange,
  selectedArtifact,
  onSelectArtifact,
  onClearArtifact,
}: WikiLeftPanelProps) {
  const { t } = useTranslation();

  return (
    <aside className="size-full min-w-0 flex flex-col p-5">
      <Tabs
        value={tab}
        onValueChange={onTabChange}
        className="w-full min-w-0 pb-5"
      >
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger value={LeftPanelTab.Contents} className="min-w-0 px-2">
            <span className="min-w-0 truncate">
              {t('knowledgeCompilation.contents')}
            </span>
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph} className="min-w-0 px-2">
            <span className="min-w-0 truncate">
              {t('knowledgeCompilation.graph')}
            </span>
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="flex-1 min-h-0 relative">
        {tab === LeftPanelTab.Contents && (
          <WikiNavBar
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
          />
        )}
        {tab === LeftPanelTab.Graph && (
          <WikiGraphPanel
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
            onClearArtifact={onClearArtifact}
          />
        )}
      </div>
    </aside>
  );
}
