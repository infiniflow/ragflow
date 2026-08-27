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
    navError,
    keywords,
    childrenMap,
    childrenErrorParents,
    structureMap,
    selectedNode,
    deleteNavLoading,
    deleteNodeLoading,
    handleKeywordsChange,
    handleNodeClick,
    handleNodeExpand,
    handleEntityClick,
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
            navError={navError}
            keywords={keywords}
            childrenMap={childrenMap}
            childrenErrorParents={childrenErrorParents}
            structureMap={structureMap}
            deleteNavLoading={deleteNavLoading}
            deleteNodeLoading={deleteNodeLoading}
            onKeywordsChange={handleKeywordsChange}
            onNodeClick={handleNodeClick}
            onNodeExpand={handleNodeExpand}
            onEntityClick={handleEntityClick}
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
                {selectedNode.doc_count !== undefined && (
                  <span className="text-xs text-text-secondary">
                    {t('knowledgeCompilation.navDocCount', {
                      count: selectedNode.doc_count,
                    })}
                  </span>
                )}
              </header>
              <div className="flex-1 min-h-0 overflow-y-auto px-4 py-3 text-sm text-text-primary space-y-4">
                <div>
                  <h4 className="text-xs font-medium text-text-secondary mb-1">
                    {t('knowledgeCompilation.description')}
                  </h4>
                  <p className="whitespace-pre-wrap">
                    {selectedNode.description ||
                      t('knowledgeCompilation.navNoDescription')}
                  </p>
                </div>
                {selectedNode.keywords && selectedNode.keywords.length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-text-secondary mb-1">
                      {t('knowledgeCompilation.navKeywords')}
                    </h4>
                    <div className="flex flex-wrap gap-1">
                      {selectedNode.keywords.map((kw, i) => (
                        <span
                          key={i}
                          className="inline-block px-2 py-0.5 rounded text-xs bg-fill-quaternary text-text-secondary"
                        >
                          {kw}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
                {selectedNode.entities && selectedNode.entities.length > 0 && (
                  <div>
                    <h4 className="text-xs font-medium text-text-secondary mb-1">
                      {t('knowledgeCompilation.navEntities')}
                    </h4>
                    <div className="flex flex-wrap gap-1">
                      {selectedNode.entities.map((entity, i) => (
                        <span
                          key={i}
                          className="inline-block px-2 py-0.5 rounded text-xs bg-fill-quaternary text-text-secondary"
                        >
                          {entity}
                        </span>
                      ))}
                    </div>
                  </div>
                )}
                {selectedNode.graph_content && (
                  <div>
                    <h4 className="text-xs font-medium text-text-secondary mb-1">
                      {t('knowledgeCompilation.navGraphContent')}
                    </h4>
                    <p className="whitespace-pre-wrap text-xs text-text-secondary">
                      {selectedNode.graph_content}
                    </p>
                  </div>
                )}
              </div>
            </section>
          ) : (
            <div className="flex-1 flex items-center justify-center text-sm text-text-secondary">
              {t('knowledgeCompilation.navSelectNode')}
            </div>
          )}
        </ResizablePanel>
      </ResizablePanelGroup>
    </Card>
  );
}
