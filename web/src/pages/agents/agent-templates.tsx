import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchAgentTemplates,
  useCreateAgent,
} from '@/hooks/use-agent-request';

import { CardContainer } from '@/components/card-container';
import { PageCloseButton } from '@/components/page-close-button';
import { SidebarPageFrame } from '@/components/sidebar-page-frame';
import { TruncatedText } from '@/components/truncated-text';
import { SearchInput } from '@/components/ui/input';
import { AgentCategory, AgentQuery } from '@/constants/agent';
import { LanguageAbbreviation } from '@/constants/common';
import { useClientSearch } from '@/hooks/use-client-search';
import { DSL, IFlowTemplate } from '@/interfaces/database/agent';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import {
  CanvasCategoryToFlowType,
  TemplateMenuItemKey,
  TemplateMenuItems,
} from './constant';
import { CreateAgentDialog } from './create-agent-dialog';
import { TemplateCard } from './template-card';
import {
  bindUnboundRetrieval,
  collectUnboundRetrievalBindings,
  bindUnboundModel,
} from './template-retrieval-binding';
import { SideBar } from './template-sidebar';

function resolveSourceCategory(value: string | null): AgentCategory {
  return value === AgentCategory.DataflowCanvas
    ? AgentCategory.DataflowCanvas
    : AgentCategory.AgentCanvas;
}

function normalizeTemplateCategory(template: IFlowTemplate): AgentCategory {
  return template.canvas_category === AgentCategory.DataflowCanvas
    ? AgentCategory.DataflowCanvas
    : AgentCategory.AgentCanvas;
}

function resolveTemplateLanguage(
  language: string,
): keyof IFlowTemplate['title'] {
  if (language === LanguageAbbreviation.Zh) {
    return 'zh';
  }
  if (language === LanguageAbbreviation.De) {
    return 'de';
  }
  return 'en';
}

export default function AgentTemplates() {
  const list = useFetchAgentTemplates();
  const { loading, createAgent } = useCreateAgent();
  const { t, i18n } = useTranslation();
  const [searchParams] = useSearchParams();
  // canvas_category is the source tab; canvas_type(s) remain the secondary
  // theme filter rendered by the sidebar.
  const sourceCategory = resolveSourceCategory(
    searchParams.get(AgentQuery.CanvasCategory),
  );
  const [selectMenuItem, setSelectMenuItem] = useState<TemplateMenuItemKey>(
    TemplateMenuItemKey.Recommended,
  );

  // The Pipeline catalog has no theme breakdown, so it renders without the
  // sidebar; the theme filter only applies to Workflow templates.
  const showSidebar = sourceCategory === AgentCategory.AgentCanvas;
  const language = resolveTemplateLanguage(i18n.language);
  const selectedMenuItem =
    TemplateMenuItems.find((item) => item.key === selectMenuItem) ??
    TemplateMenuItems[0];
  const contentTitle = showSidebar
    ? t(`flow.${selectedMenuItem.labelKey}`)
    : t('flow.tabList.ingestionPipeline');

  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const [template, setTemplate] = useState<IFlowTemplate>();

  const showModal = useCallback(
    (record: IFlowTemplate) => {
      setTemplate(record);
      showCreatingModal();
    },
    [showCreatingModal],
  );

  const { navigateToAgent, navigateToAgents } = useNavigatePage();

  const handleClose = useCallback(() => {
    navigateToAgents(sourceCategory);
  }, [navigateToAgents, sourceCategory]);

  const handleOk = useCallback(
    async (payload: any) => {
      const dsl = template?.dsl;
      const canvasCategory = template
        ? normalizeTemplateCategory(template)
        : AgentCategory.AgentCanvas;
      const memoryIds: string[] = payload?.memory_ids ?? [];
      const modelId: string = payload?.llm_id ?? '';
      const datasetBlocks = collectUnboundRetrievalBindings(dsl).datasetBlocks;
      const datasetBindings = Object.fromEntries(
        datasetBlocks.map((block, index) => [
          block.blockId,
          datasetBlocks.length === 1
            ? (payload?.dataset_ids ?? [])
            : (payload?.dataset_bindings?.[index] ?? []),
        ]),
      );
      let boundDsl: DSL | Record<string, any> | undefined =
        dsl &&
        (Object.values(datasetBindings).some((ids) => ids.length > 0) ||
          memoryIds.length > 0)
          ? bindUnboundRetrieval(dsl, datasetBindings, memoryIds)
          : dsl;
      boundDsl = bindUnboundModel(boundDsl, modelId);

      const ret = await createAgent({
        title: payload.name,
        dsl: boundDsl,
        avatar: template?.avatar,
        canvas_category: canvasCategory,
      });

      if (ret?.code === 0) {
        hideCreatingModal();
        if (canvasCategory === AgentCategory.DataflowCanvas) {
          navigateToAgent(ret.data.id, AgentCategory.DataflowCanvas)();
        } else {
          navigateToAgent(ret.data.id)();
        }
      }
    },
    [hideCreatingModal, navigateToAgent, createAgent, template],
  );

  const handleSidebarChange = useCallback((keyword: TemplateMenuItemKey) => {
    setSelectMenuItem(keyword);
  }, []);

  const tempListFilter = useMemo(() => {
    const scoped = list.filter(
      (item) => normalizeTemplateCategory(item) === sourceCategory,
    );
    if (!showSidebar || !selectMenuItem) {
      return scoped;
    }
    const selectedCanvasType = selectMenuItem.toLocaleLowerCase();
    return scoped.filter((item) => {
      if (Array.isArray(item.canvas_types) && item.canvas_types.length > 0) {
        return item.canvas_types.some(
          (canvasType) =>
            typeof canvasType === 'string' &&
            canvasType.toLocaleLowerCase() === selectedCanvasType,
        );
      }
      return item.canvas_type?.toLocaleLowerCase() === selectedCanvasType;
    });
  }, [selectMenuItem, list, sourceCategory, showSidebar]);

  const templateSearchFields = useMemo(
    () => [(item: IFlowTemplate) => item.title[language]],
    [language],
  );
  const { filteredData, searchKeyword, handleSearchChange } =
    useClientSearch<IFlowTemplate>({
      data: tempListFilter,
      searchFields: templateSearchFields,
    });

  return (
    <>
      <SidebarPageFrame
        closeButton={<PageCloseButton onClick={handleClose} />}
        sidebar={
          showSidebar ? (
            <SideBar
              change={handleSidebarChange}
              selected={selectMenuItem}
            ></SideBar>
          ) : undefined
        }
      >
        <main className="flex h-full min-h-0 min-w-0 flex-1 flex-col">
          <header className="flex shrink-0 justify-center">
            <SearchInput
              value={searchKeyword}
              onChange={handleSearchChange}
              className="h-10 rounded-full"
              rootClassName="w-[400px] min-w-0 max-w-full"
              role="searchbox"
            />
          </header>

          <TruncatedText
            as="h2"
            className="mb-5 mt-8 min-w-0 truncate text-xl font-medium text-text-primary"
            tooltip={contentTitle}
          >
            {contentTitle}
          </TruncatedText>

          <CardContainer className="min-h-0 flex-1 overflow-auto sm:grid-cols-1 md:grid-cols-1 lg:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-2">
            {filteredData.map((item) => (
              <TemplateCard
                key={item.id}
                data={item}
                language={language}
                showModal={showModal}
              />
            ))}
          </CardContainer>
        </main>
      </SidebarPageFrame>

      {creatingVisible && (
        <CreateAgentDialog
          loading={loading}
          visible={creatingVisible}
          hideModal={hideCreatingModal}
          selectedType={
            CanvasCategoryToFlowType[
              template ? normalizeTemplateCategory(template) : sourceCategory
            ]
          }
          template={template}
          onOk={handleOk}
        ></CreateAgentDialog>
      )}
    </>
  );
}
