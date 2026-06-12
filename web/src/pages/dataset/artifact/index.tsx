import { ArtifactGraph } from './artifact-graph';
import { ArtifactList } from './artifact-list';
import { ArtifactViewer } from './artifact-viewer';
import { useArtifactView } from './hooks/use-artifact-state';

/**
 * Dataset > Artifact tab.
 *
 * Layout:
 *   ┌────────────┬──────────────────────────────────────┐
 *   │ list (38%) │ markdown viewer OR graph view (62%)  │
 *   └────────────┴──────────────────────────────────────┘
 * Side-by-side flex-row split — full viewport height with internal scroll
 * in each panel so the header / dataset title stay anchored.
 *
 * The selection state lives in the URL (`?page_type=…&slug=…&view=…`) so
 * both sub-components share a single source of truth — the markdown link
 * renderer can navigate to other pages by updating the query string, and
 * the right pane swaps between viewer / graph based on ``?view``.
 */
export default function DatasetArtifactPage() {
  const { view, setView } = useArtifactView();
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
    </div>
  );
}
