import ArtifactForceGraph from '@/components/artifact-force-graph';
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useFetchArtifactGraph } from '@/hooks/use-knowledge-request';
import { IArtifact, IArtifactGraphEntity } from '@/interfaces/database/dataset';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { LeftPanelTab } from '../constants';
import { useWikiClear } from './hooks/use-wiki-clear';
import { WikiNavBar } from './wiki-nav-bar';

const mapNodeToValue = (node: IArtifactGraphEntity) => ({
  slug: node.slug,
  title: node.name,
  page_type: node.type,
});

type WikiLeftPanelProps = {
  tab: LeftPanelTab;
  onTabChange: (value: string) => void;
  selectedArtifact: IArtifact | null;
  onSelectArtifact: (artifact: IArtifact) => void;
  onClearWiki?: () => void;
};

export function WikiLeftPanel({
  tab,
  onTabChange,
  selectedArtifact,
  onSelectArtifact,
  onClearWiki,
}: WikiLeftPanelProps) {
  const { t } = useTranslation();
  const { data } = useFetchArtifactGraph(undefined, {
    enabled: tab === LeftPanelTab.Graph,
  });
  const { open, setOpen, handleConfirm, loading } = useWikiClear({
    onClearWiki,
  });

  return (
    <aside className="size-full flex flex-col p-5">
      <section className="flex items-center justify-between pb-5">
        <Tabs value={tab} onValueChange={onTabChange}>
          <TabsList className="grid grid-cols-2 w-80">
            <TabsTrigger value={LeftPanelTab.Contents}>
              {t('knowledgeDetails.contents')}
            </TabsTrigger>
            <TabsTrigger value={LeftPanelTab.Graph}>
              {t('knowledgeDetails.graph')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <ConfirmDeleteDialog
          open={open}
          onOpenChange={setOpen}
          title={t('knowledgeDetails.clearWikiTitle')}
          content={{ title: t('knowledgeDetails.clearWikiDescription') }}
          onOk={handleConfirm}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={loading}
            data-testid="wiki-clear-trigger"
          >
            <Trash2 className="size-[1em]" />
          </Button>
        </ConfirmDeleteDialog>
      </section>

      <div className="flex-1 min-h-0 relative">
        {tab === LeftPanelTab.Contents && (
          <WikiNavBar
            selectedArtifact={selectedArtifact}
            onSelectArtifact={onSelectArtifact}
          />
        )}
        {tab === LeftPanelTab.Graph && (
          <ArtifactForceGraph
            data={data}
            show
            mapNodeToValue={mapNodeToValue}
            onNodeClick={onSelectArtifact}
          />
        )}
      </div>
    </aside>
  );
}
