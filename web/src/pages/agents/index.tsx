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
      {(!data?.length || data?.length <= 0) && !searchString && (
        <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
          <EmptyAppCard
            showIcon
            size="large"
            className="w-[480px] p-14 !cursor-default"
            isSearch={!!searchString}
            type={EmptyCardType.Agent}
            // onClick={() => showCreatingModal()}
          >
            <div className="flex flex-col gap-y-5 text-text-secondary text-sm pt-5">
              <div
                className="flex items-center gap-x-2 hover:text-text-primary cursor-pointer"
                onClick={showCreatingModal}
              >
                <Clipboard size={14} />
                {t('flow.createFromBlank')}
              </div>
              <div
                className="flex items-center gap-x-2 hover:text-text-primary cursor-pointer"
                onClick={navigateToAgentTemplates}
              >
                <ClipboardPlus size={14} />
                {t('flow.createFromTemplate')}
              </div>
              <div
                className="flex items-center gap-x-2 hover:text-text-primary cursor-pointer"
                onClick={handleImportJson}
              >
                <FileInput size={14} />
                {t('flow.importJsonFile')}
              </div>
            </div>
          </EmptyAppCard>
        </div>
      )}
      <section className="flex flex-col w-full flex-1">
        {(!!data?.length || searchString) && (
          <>
            <div className="px-8 pt-8 ">
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
                  <DropdownMenuTrigger>
                    <Button>
                      <Plus className="h-4 w-4" />
                      {t('flow.createGraph')}
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent>
                    <DropdownMenuItem
                      justifyBetween={false}
                      onClick={showCreatingModal}
                    >
                      <Clipboard />
                      {t('flow.createFromBlank')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      justifyBetween={false}
                      onClick={navigateToAgentTemplates}
                    >
                      <ClipboardPlus />
                      {t('flow.createFromTemplate')}
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      justifyBetween={false}
                      onClick={handleImportJson}
                    >
                      <FileInput />
                      {t('flow.importJsonFile')}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </ListFilterBar>
            </div>
            {(!data?.length || data?.length <= 0) && searchString && (
              <div className="flex w-full items-center justify-center h-[calc(100vh-164px)]">
                <EmptyAppCard
                  showIcon
                  size="large"
                  className="w-[480px] p-14"
                  isSearch={!!searchString}
                  type={EmptyCardType.Agent}
                  onClick={() => showCreatingModal()}
                />
              </div>
            )}
            <div className="flex-1 overflow-auto">
              <CardContainer className="max-h-[calc(100dvh-280px)] overflow-auto px-8">
                {data.map((x) => {
                  return (
                    <AgentCard
                      key={x.id}
                      data={x}
                      showAgentRenameModal={showAgentRenameModal}
                    ></AgentCard>
                  );
                })}
              </CardContainer>
            </div>
            <div className="mt-8 px-8 pb-8">
              <RAGFlowPagination
                {...pick(pagination, 'current', 'pageSize')}
                total={pagination.total}
                onChange={handlePageChange}
              ></RAGFlowPagination>
            </div>
          </>
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
      </section>
    </>
  );
}
