import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { TreeView } from '@/components/ui/tree-view';
import { GenerateStatus } from '@/constants/knowledge';
import { ITraceInfo, useGenerateStatus } from '@/hooks/use-dataset-generate';
import {
  DatasetNavList,
  DatasetNavNode,
} from '@/interfaces/database/dataset-nav';
import { IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { cn } from '@/lib/utils';
import { CircleX, FileText, Folder, Loader2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UpdateLogSheet } from './update-log-sheet';
import { buildNavTreeData, NavEntityClickHandler } from './utils/nav-tree';

// TreeView only computes expandedItemIds when initialSelectedItemId is truthy;
// combined with expandAll, any truthy id makes every branch mount open. A
// sentinel that matches no real node forces expand-all without highlighting any
// row as selected.
const NavExpandAllSentinelId = '__nav-tree-expand-all-sentinel__';

type NavTreeLeftPanelProps = {
  navList: DatasetNavList | null;
  navLoading: boolean;
  navError?: boolean;
  keywords: string;
  // The debounced filter applied to the nav/children/graph requests. Used as
  // the TreeView key so a filter change remounts the tree: expansion state is
  // uncontrolled per node and onExpand only fires on opening, so without a
  // remount an already-open node whose cached children were dropped would sit
  // on the loading placeholder forever.
  activeKeywords: string;
  childrenMap: Record<string, DatasetNavNode[]>;
  childrenErrorParents?: Record<string, boolean>;
  structureMap: Record<string, IStructureGraphTemplate[]>;
  traceData?: ITraceInfo;
  onKeywordsChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onNodeClick: (node: DatasetNavNode, parentName: string | null) => void;
  onNodeExpand: (node: DatasetNavNode) => void;
  onEntityClick: NavEntityClickHandler;
};

export function NavTreeLeftPanel({
  navList,
  navLoading,
  navError = false,
  keywords,
  activeKeywords,
  childrenMap,
  childrenErrorParents = {},
  structureMap,
  traceData,
  onKeywordsChange,
  onNodeClick,
  onNodeExpand,
  onEntityClick,
}: NavTreeLeftPanelProps) {
  const { t } = useTranslation();

  const { status: compileStatus } = useGenerateStatus(traceData);
  const [logSheetOpen, setLogSheetOpen] = useState(false);
  // An incremental compile is running while a tree is already on screen —
  // surface it as a log entry point in the header (the full-view placeholder
  // covers the first compile, when no tree exists).
  const compiling =
    compileStatus === GenerateStatus.Running ||
    compileStatus === GenerateStatus.Failed;
  const compileFailed = compiling && compileStatus === GenerateStatus.Failed;

  const handleOpenLogSheet = useCallback(() => {
    setLogSheetOpen(true);
  }, []);

  const treeData = useMemo(
    () =>
      buildNavTreeData(navList?.items, {
        childrenMap,
        childrenErrorParents,
        structureMap,
        // A search response is a pruned forest (hits + the cluster path above
        // them), so it is nested from the payload instead of being fetched
        // branch by branch.
        searchMode: !!activeKeywords,
        onNodeClick,
        onNodeExpand,
        onEntityClick,
        loadingPlaceholder: t('knowledgeCompilation.navLoading'),
        errorPlaceholder: t('knowledgeCompilation.navChildLoadFailed'),
      }),
    [
      navList?.items,
      activeKeywords,
      childrenMap,
      childrenErrorParents,
      structureMap,
      onNodeClick,
      onNodeExpand,
      onEntityClick,
      t,
    ],
  );

  return (
    <aside className="size-full flex flex-col">
      <section className="flex items-center justify-between px-3 pt-3">
        <span className="text-sm font-medium text-text-primary">
          {t('knowledgeCompilation.navTitle')} ({navList?.total ?? 0})
        </span>
        {compiling && (
          <Button
            variant="ghost"
            size="sm"
            onClick={handleOpenLogSheet}
            data-testid="nav-compile-log-trigger"
            className={cn({ 'text-state-error': compileFailed })}
          >
            {compileFailed ? <CircleX /> : <Loader2 className="animate-spin" />}
            <span
              className="max-w-56 truncate"
              title={compileFailed ? traceData?.compilationError : undefined}
            >
              {compileFailed
                ? traceData?.compilationError || t('message.operated')
                : t('knowledgeCompilation.compiling')}
            </span>
          </Button>
        )}
      </section>

      <div className="px-3 pt-2">
        <SearchInput value={keywords} onChange={onKeywordsChange} />
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-1 pt-2 pb-3">
        {navLoading && treeData.length === 0 ? (
          <div className="py-8 flex justify-center">
            <Spin size="small" />
          </div>
        ) : treeData.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-secondary">
            {t(
              navError
                ? 'knowledgeCompilation.navLoadFailed'
                : 'knowledgeCompilation.navEmpty',
            )}
          </div>
        ) : (
          <>
            {navError ? (
              <div className="px-2 pb-2 text-center text-sm text-text-secondary">
                {t('knowledgeCompilation.navLoadFailed')}
              </div>
            ) : null}
            <TreeView
              key={activeKeywords}
              data={treeData}
              // Search: mount the matched branches open (sentinel trick above).
              expandAll={!!activeKeywords}
              initialSelectedItemId={
                activeKeywords ? NavExpandAllSentinelId : undefined
              }
              expandOnRowClick={false}
              defaultNodeIcon={Folder}
              defaultLeafIcon={FileText}
            />
          </>
        )}
      </div>

      <UpdateLogSheet
        open={logSheetOpen}
        onOpenChange={setLogSheetOpen}
        data={traceData}
        title={t('knowledgeCompilation.navLogTitle')}
      />
    </aside>
  );
}
