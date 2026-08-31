import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Spin } from '@/components/ui/spin';
import { TreeView } from '@/components/ui/tree-view';
import {
  DatasetNavList,
  DatasetNavNode,
} from '@/interfaces/database/dataset-nav';
import { IStructureGraphTemplate } from '@/interfaces/database/document-structure';
import { FileText, Folder, Trash2 } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { buildNavTreeData, NavEntityClickHandler } from './utils/nav-tree';

type NavNodeDeleteActionProps = {
  name: string;
  parentName: string | null;
  deleteLoading: boolean;
  onDelete: (name: string, parentName: string | null) => void;
};

function NavNodeDeleteAction({
  name,
  parentName,
  deleteLoading,
  onDelete,
}: NavNodeDeleteActionProps) {
  const { t } = useTranslation();

  const handleTriggerClick = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      // TreeView does not guard action clicks: without this the row would
      // also get selected and a branch row would toggle its accordion.
      e.stopPropagation();
    },
    [],
  );

  const handleConfirmDelete = useCallback(() => {
    onDelete(name, parentName);
  }, [name, parentName, onDelete]);

  return (
    <ConfirmDeleteDialog
      title={t('knowledgeCompilation.navDeleteNodeTitle')}
      content={{ title: t('knowledgeCompilation.navDeleteNodeDescription') }}
      onOk={handleConfirmDelete}
    >
      <Button
        variant="ghost"
        size="icon-sm"
        disabled={deleteLoading}
        onClick={handleTriggerClick}
        // TreeActions keeps actions always visible on the selected row;
        // hide again so the button only appears while hovering the row.
        // `hidden` (not opacity-0) so no invisible click target remains.
        className="hidden group-hover:inline-flex"
      >
        <Trash2 />
      </Button>
    </ConfirmDeleteDialog>
  );
}

type NavTreeLeftPanelProps = {
  navList: DatasetNavList | null;
  navLoading: boolean;
  navError?: boolean;
  keywords: string;
  childrenMap: Record<string, DatasetNavNode[]>;
  childrenErrorParents?: Record<string, boolean>;
  structureMap: Record<string, IStructureGraphTemplate[]>;
  deleteNavLoading: boolean;
  deleteNodeLoading: boolean;
  onKeywordsChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onNodeClick: (node: DatasetNavNode, parentName: string | null) => void;
  onNodeExpand: (node: DatasetNavNode) => void;
  onEntityClick: NavEntityClickHandler;
  onDeleteAll: () => void;
  onDeleteNode: (name: string, parentName: string | null) => void;
};

export function NavTreeLeftPanel({
  navList,
  navLoading,
  navError = false,
  keywords,
  childrenMap,
  childrenErrorParents = {},
  structureMap,
  deleteNavLoading,
  deleteNodeLoading,
  onKeywordsChange,
  onNodeClick,
  onNodeExpand,
  onEntityClick,
  onDeleteAll,
  onDeleteNode,
}: NavTreeLeftPanelProps) {
  const { t } = useTranslation();

  const renderNavActions = useCallback(
    (node: DatasetNavNode, parentName: string | null) => (
      <NavNodeDeleteAction
        name={node.name}
        parentName={parentName}
        deleteLoading={deleteNodeLoading}
        onDelete={onDeleteNode}
      />
    ),
    [deleteNodeLoading, onDeleteNode],
  );

  const treeData = useMemo(
    () =>
      buildNavTreeData(navList?.items, {
        childrenMap,
        childrenErrorParents,
        structureMap,
        getActions: renderNavActions,
        onNodeClick,
        onNodeExpand,
        onEntityClick,
        loadingPlaceholder: t('knowledgeCompilation.navLoading'),
        errorPlaceholder: t('knowledgeCompilation.navChildLoadFailed'),
      }),
    [
      navList?.items,
      childrenMap,
      childrenErrorParents,
      structureMap,
      renderNavActions,
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
        {treeData.length > 0 && (
          <ConfirmDeleteDialog
            title={t('knowledgeCompilation.navDeleteAllTitle')}
            content={{
              title: t('knowledgeCompilation.navDeleteAllDescription'),
            }}
            onOk={onDeleteAll}
          >
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={deleteNavLoading}
              data-testid="nav-tree-clear-trigger"
            >
              <Trash2 />
            </Button>
          </ConfirmDeleteDialog>
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
              data={treeData}
              expandOnRowClick={false}
              defaultNodeIcon={Folder}
              defaultLeafIcon={FileText}
            />
          </>
        )}
      </div>
    </aside>
  );
}
