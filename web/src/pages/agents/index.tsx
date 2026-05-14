import { CardContainer } from '@/components/card-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import ListFilterBar from '@/components/list-filter-bar';
import { RenameDialog } from '@/components/rename-dialog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentListByPage } from '@/hooks/use-agent-request';
import { Routes } from '@/routes';
import { t } from 'i18next';
import { pick } from 'lodash';
import { Clipboard, ClipboardPlus, FileInput, Plus } from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router';
import { AgentCard } from './agent-card';
import { CreateAgentDialog } from './create-agent-dialog';
import { useCreateAgentOrPipeline } from './hooks/use-create-agent';
import { useSelectFilters } from './hooks/use-selelct-filters';
import { UploadAgentDialog } from './upload-agent-dialog';
import { useHandleImportJsonFile } from './use-import-json';
import { useRenameAgent } from './use-rename-agent';

export default function Agents() {
  const {
    data,
    pagination,
    setPagination,
    searchString,
    handleInputChange,
    filterValue,
    handleFilterSubmit,
  } = useFetchAgentListByPage();

  const { navigateToAgentTemplates } = useNavigatePage();

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
  } = useHandleImportJsonFile();

  const filters = useSelectFilters();

  const handlePageChange = useCallback(
    (page: number, pageSize?: number) => {
      setPagination({ page, pageSize });
    },
    [setPagination],
  );
  const [searchUrl, setSearchUrl] = useSearchParams();
  const isCreate = searchUrl.get('isCreate') === 'true';

  useEffect(() => {
    if (isCreate) {
      showCreatingModal();
      searchUrl.delete('isCreate');
      setSearchUrl(searchUrl);
    }
  }, [isCreate, showCreatingModal, searchUrl, setSearchUrl]);

  return (
    <>
      {data?.length || searchString ? (
        <article className="size-full flex flex-col" data-testid="agents-list">
          <header className="px-5 pt-8 mb-4">
            <ListFilterBar
              title={t('flow.agents')}
              searchString={searchString}
              onSearchChange={handleInputChange}
              icon="agents"
              filters={filters}
              onChange={handleFilterSubmit}
              value={filterValue}
            >
              <DropdownMenu>
                <DropdownMenuTrigger data-testid="create-agent">
                  <Button>
                    <Plus className="size-[1em]" />
                    {t('flow.createGraph')}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent data-testid="agent-create-menu">
                  <DropdownMenuItem
                    justifyBetween={false}
                    onClick={showCreatingModal}
                  >
                    <Clipboard />
                    {t('flow.createFromBlank')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    justifyBetween={false}
                    onClick={() => navigateToAgentTemplates()}
                  >
                    <ClipboardPlus />
                    {t('flow.createFromTemplate')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    data-testid="agent-import-json"
                    justifyBetween={false}
                    onClick={handleImportJson}
                  >
                    <FileInput />
                    {t('flow.importJsonFile')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </ListFilterBar>
          </header>

          {data.length ? (
            <>
              <CardContainer className="flex-1 overflow-auto px-5">
                {data.map((x) => {
                  return (
                    <AgentCard
                      key={x.id}
                      data={x}
                      showAgentRenameModal={showAgentRenameModal}
                    />
                  );
                })}
              </CardContainer>

              <footer className="mt-4 px-5 pb-5">
                <RAGFlowPagination
                  {...pick(pagination, 'current', 'pageSize')}
                  total={pagination.total}
                  onChange={handlePageChange}
                />
              </footer>
            </>
          ) : (
            <div className="flex-1 flex items-center justify-center">
              <EmptyAppCard
                showIcon
                size="large"
                className="w-[480px] p-14"
                isSearch
                type={EmptyCardType.Agent}
                onClick={() => showCreatingModal()}
              />
            </div>
          )}
        </article>
      ) : (
        <article
          className="size-full flex items-center justify-center"
          data-testid="agents-list"
        >
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14 !cursor-default"
            type={EmptyCardType.Agent}
            tabIndex={-1}
            // onClick={() => showCreatingModal()}
          >
            <ul className="flex flex-col gap-y-5 text-text-secondary text-sm pt-5">
              <li data-testid="agents-empty-create">
                <Button
                  variant="static"
                  size="auto"
                  onClick={showCreatingModal}
                >
                  <Clipboard className="size-[1em]" />
                  {t('flow.createFromBlank')}
                </Button>
              </li>

              <li>
                <Button
                  asLink
                  variant="static"
                  size="auto"
                  to={Routes.AgentTemplates}
                >
                  <ClipboardPlus className="size-[1em]" />
                  {t('flow.createFromTemplate')}
                </Button>
              </li>

              <li>
                <Button variant="static" size="auto" onClick={handleImportJson}>
                  <FileInput className="size-[1em]" />
                  {t('flow.importJsonFile')}
                </Button>
              </li>
            </ul>
          </EmptyAppCard>
        </article>
      )}

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
          shouldChooseAgent
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
