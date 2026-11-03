import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { ListDeletionKey } from '@/constants/list-deletion';
import { useGoToPreviousPageOnEmpty } from '@/hooks/logic-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { useDeleteCompilationTemplateGroup } from '@/hooks/use-compilation-template-group-request';
import { AgentListItemType } from '@/interfaces/database/agent';
import { pick } from 'lodash';
import { useCallback, useEffect, useState } from 'react';
import { AgentCard } from './agent-card';
import {
  AgentCreateMenu,
  AgentsEmptyStateActions,
} from './agent-create-actions';
import { AgentTabGroup } from './agent-tab-group';
import { BuiltinCatalogProbe } from './builtin-catalog-probe';
import { BuiltinPipelineSection } from './builtin-pipeline-section';
import {
  resolveAgentsEmptyState,
  shouldShowBuiltinForCategory,
} from './builtin-pipeline-list';
import { CompilationTemplateCard } from './compilation-template-card';
import { FlowType } from './constant';
import { CreateAgentDialog } from './create-agent-dialog';
import { useAgentCreateActions } from './hooks/use-agent-create-actions';
import { useAgentTab } from './hooks/use-agent-tab';
import { useCreateAgentOrPipeline } from './hooks/use-create-agent';
import { useSelectFilters } from './hooks/use-select-filters';
import { UploadAgentDialog } from './upload-agent-dialog';
import { useHandleImportJsonFile } from './use-import-json';
import { useRenameAgent } from './use-rename-agent';

export default function Agents() {
  const {
    searchUrl,
    setSearchUrl,
    activeCategory,
    activeType,
    handleTabChange,
  } = useAgentTab();

  const {
    data,
    loading: listLoading,
    pagination,
    setPagination,
    searchString,
    setSearchString,
    handleInputChange,
    filterValue,
    setFilterValue,
    handleFilterSubmit,
    checkValue,
    debouncedSearchString,
  } = useFetchAgentListByPage(activeCategory);

  // Built-in pipelines are parsing methods => dataflow canvases, so the section
  // (and its catalog request) is mounted only for the Pipeline tab.
  const builtinVisible = shouldShowBuiltinForCategory(activeCategory);

  // Lifted filtered built-in catalog state, reported by BuiltinCatalogProbe so
  // the empty-state decision can use *matched* built-in items rather than just
  // category eligibility.
  const [builtinState, setBuiltinState] = useState<{
    length: number;
    loading: boolean;
  }>({ length: 0, loading: false });

  useEffect(() => {
    if (!builtinVisible) {
      setBuiltinState({ length: 0, loading: false });
    }
  }, [builtinVisible]);

  const {
    agentRenameLoading,
    initialAgentName,
    onAgentRenameOk,
    agentRenameVisible,
    hideAgentRenameModal,
    showAgentRenameModal,
  } = useRenameAgent();

  const {
    creatingVisible,
    hideCreatingModal,
    showCreatingModal,
    loading,
    handleCreateAgentOrPipeline,
  } = useCreateAgentOrPipeline();

  const {
    handleImportJson,
    fileUploadVisible,
    onFileUploadOk,
    hideFileUploadModal,
  } = useHandleImportJsonFile(activeCategory);

  const createActions = useAgentCreateActions(
    activeType,
    activeCategory,
    showCreatingModal,
    handleImportJson,
  );

  const { deleteGroup } = useDeleteCompilationTemplateGroup();

  const filters = useSelectFilters(activeCategory);

  useEffect(() => {
    checkValue(filters);
  }, [filters, checkValue]);

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );
  useGoToPreviousPageOnEmpty(data?.length, listLoading, {
    deletionKey: ListDeletionKey.AgentList,
    searchString,
    setSearchString,
    filterValue,
    setFilterValue,
  });

  const { navigateToCompilationEditor } = useNavigatePage();

  const handleCreateCompilation = useCallback(() => {
    navigateToCompilationEditor();
  }, [navigateToCompilationEditor]);

  const handleEditCompilation = useCallback(
    (id: string) => () => {
      navigateToCompilationEditor(id);
    },
    [navigateToCompilationEditor],
  );

  const handleDeleteCompilation = useCallback(
    async (id: string) => {
      await deleteGroup(id);
    },
    [deleteGroup],
  );

  const isCreate = searchUrl.get('isCreate') === 'true';

  useEffect(() => {
    if (isCreate) {
      showCreatingModal();
      const nextParams = new URLSearchParams(searchUrl);
      nextParams.delete('isCreate');
      setSearchUrl(nextParams, { replace: true });
    }
  }, [isCreate, showCreatingModal, searchUrl, setSearchUrl]);

  const emptyState = resolveAgentsEmptyState({
    dataLength: data.length,
    builtinItemsLength: builtinState.length,
    searchString: debouncedSearchString,
    listLoading,
    builtinVisible,
    builtinLoading: builtinState.loading,
  });

  return (
    <>
      {builtinVisible && (
        <BuiltinCatalogProbe
          canvasCategory={activeCategory}
          searchString={debouncedSearchString}
          onReport={setBuiltinState}
        />
      )}
      <article
        className="size-full min-w-0 flex flex-col"
        data-testid="agents-list"
      >
        <header className="mb-4 min-w-0 px-5 pt-8">
          <ListFilterBar
            searchString={searchString}
            onSearchChange={handleInputChange}
            filters={filters}
            onChange={handleFilterSubmit}
            value={filterValue}
            leftPanel={
              <AgentTabGroup
                activeCategory={activeCategory}
                onTabChange={handleTabChange}
              />
            }
          >
            <AgentCreateMenu
              actions={createActions}
              directOnClick={
                activeType === FlowType.Compiler
                  ? handleCreateCompilation
                  : undefined
              }
            />
          </ListFilterBar>
        </header>

        {emptyState === 'content' ? (
          <>
            <CardContainer className="flex-1 overflow-auto px-5">
              {data.map((x) =>
                x.type === AgentListItemType.CompilationTemplateGroup ? (
                  <CompilationTemplateCard
                    key={x.id}
                    data={x}
                    onClick={handleEditCompilation(x.id)}
                    onDelete={handleDeleteCompilation}
                  />
                ) : (
                  <AgentCard
                    key={x.id}
                    data={x}
                    showAgentRenameModal={showAgentRenameModal}
                  />
                ),
              )}

              {builtinVisible && (
                <BuiltinPipelineSection
                  canvasCategory={activeCategory}
                  searchString={debouncedSearchString}
                  showDivider={data.length > 0}
                />
              )}
            </CardContainer>

            {data.length > 0 && (
              <footer className="mt-4 px-5 pb-5">
                <RAGFlowPagination
                  {...pick(pagination, 'current', 'pageSize')}
                  total={pagination.total}
                  onChange={handlePageChange}
                />
              </footer>
            )}
          </>
        ) : emptyState === 'search-empty' ? (
          <div className="flex-1 flex items-center justify-center">
            <EmptyAppCard
              showIcon
              size="large"
              className="w-[480px] p-14"
              isSearch
              testId="agents-search-empty"
              type={EmptyCardType.Agent}
              onClick={showCreatingModal}
            />
          </div>
        ) : emptyState === 'loading' ? null : (
          <div className="flex-1 flex items-center justify-center">
            <EmptyAppCard
              showIcon
              size="large"
              className="w-[480px] p-14 !cursor-default"
              type={EmptyCardType.Agent}
              tabIndex={-1}
            >
              <AgentsEmptyStateActions actions={createActions} />
            </EmptyAppCard>
          </div>
        )}
      </article>

      {agentRenameVisible && (
        <RenameDialog
          hideModal={hideAgentRenameModal}
          onOk={onAgentRenameOk}
          initialName={initialAgentName}
          loading={agentRenameLoading}
        ></RenameDialog>
      )}
      {creatingVisible && (
        <CreateAgentDialog
          loading={loading}
          visible={creatingVisible}
          hideModal={hideCreatingModal}
          selectedType={activeType}
          onOk={handleCreateAgentOrPipeline}
        ></CreateAgentDialog>
      )}
      {fileUploadVisible && (
        <UploadAgentDialog
          hideModal={hideFileUploadModal}
          onOk={onFileUploadOk}
        ></UploadAgentDialog>
      )}
    </>
  );
}
