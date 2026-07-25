import { Card } from '@/components/ui/card';
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable';
import { useTranslation } from 'react-i18next';

import { useCompilationNav } from './hooks/use-compilation-nav';
import { NavTreeLeftPanel } from './nav-tree-left-panel';

export function NavTreeView() {
  const { t } = useTranslation();
  const {
    navList,
    navLoading,
    childrenMap,
    selectedNode,
    deleteNavLoading,
    deleteNodeLoading,
    handleParentClick,
    handleChildClick,
    handleDeleteAll,
    handleDeleteNode,
  } = useCompilationNav();

  return (
    <Card className="flex-1 min-h-0 overflow-hidden flex border-border-button rounded-xl flex-col">
      <ResizablePanelGroup direction="horizontal" className="flex-1">
        <ResizablePanel defaultSize={33} minSize={20} maxSize={50}>
          <NavTreeLeftPanel
            navList={navList}
            navLoading={navLoading}
            childrenMap={childrenMap}
            deleteNavLoading={deleteNavLoading}
            deleteNodeLoading={deleteNodeLoading}
            onParentClick={handleParentClick}
            onChildClick={handleChildClick}
            onDeleteAll={handleDeleteAll}
            onDeleteNode={handleDeleteNode}
          />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel className="flex flex-col">
          {selectedNode ? (
            <section className="flex flex-col h-full">
              <header className="px-4 py-3 border-b border-border-button space-y-1">
                <h3 className="text-sm font-medium text-text-primary">
                  {selectedNode.name}
                </h3>
                <span className="text-xs text-text-secondary">
                  {t('datasetNav.docCount', { count: selectedNode.doc_count })}
                </span>
              </header>
              <div className="flex-1 min-h-0 overflow-y-auto px-4 py-3 text-sm text-text-primary whitespace-pre-wrap">
                {selectedNode.description || t('datasetNav.noDescription')}
              </div>
            </section>
          ) : (
            <div className="flex-1 flex items-center justify-center text-sm text-text-secondary">
              {t('datasetNav.selectNode')}
            </div>
          )}
        </ResizablePanel>
      </ResizablePanelGroup>
    </Card>
  );
}
