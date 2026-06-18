import { ArtifactCommitsPane } from './artifact-commits-pane';
import { ArtifactGraph } from './artifact-graph';
import { ArtifactList } from './artifact-list';
import { ArtifactViewer } from './artifact-viewer';
import {
  useArtifactSelection,
  useArtifactView,
} from './hooks/use-artifact-state';

/**
 * Dataset > Artifact tab.
 *
 * Layout (page view):
 *   ┌────────────┬───────────────────────────┬───────────────┐
 *   │ list (38%) │ markdown viewer (1fr)     │ history 320px │
 *   └────────────┴───────────────────────────┴───────────────┘
 * Layout (graph view): the right history pane is hidden — there is no
 * per-page selection while the graph is open. The history pane is also
 * hidden when no page is selected (avoid an out-of-context empty column).
 */
export default function DatasetArtifactPage() {
  const { view, setView } = useArtifactView();
  const { selected } = useArtifactSelection();
  const showHistory = view !== 'graph' && !!selected;
  return (
    <div className="flex flex-row h-full min-h-0 bg-bg-base">
      <div className="basis-[38%] shrink-0 min-w-0">
        <ArtifactList />
      </div>
      <div className="flex-1 min-w-0 flex flex-col">
        {view === 'graph' ? (
          <ArtifactGraph onClose={() => setView('page')} />
        ) : (
          <ArtifactViewer />
        )}
      </div>
      {showHistory ? (
        <ArtifactCommitsPane
          pageType={selected?.pageType ?? null}
          slug={selected?.slug ?? null}
        />
      ) : null}
    </div>
  );
}
