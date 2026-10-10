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
    <aside className="size-full flex flex-col p-5">
      <Tabs value={tab} onValueChange={onTabChange} className="pb-5">
        <TabsList className="grid grid-cols-2 w-80">
          <TabsTrigger value={LeftPanelTab.Contents}>
            {t('knowledgeCompilation.contents')}
          </TabsTrigger>
          <TabsTrigger value={LeftPanelTab.Graph}>
            {t('knowledgeCompilation.graph')}
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
