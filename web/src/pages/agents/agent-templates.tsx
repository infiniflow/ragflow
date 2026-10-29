import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchAgentTemplates,
  useCreateAgent,
} from '@/hooks/use-agent-request';

import { CardContainer } from '@/components/card-container';
import { AgentCategory, AgentQuery } from '@/constants/agent';
import { DSL, IFlowTemplate } from '@/interfaces/database/agent';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { CanvasCategoryToFlowType } from './constant';
import { CreateAgentDialog } from './create-agent-dialog';
import { TemplateCard } from './template-card';
import {
  bindUnboundRetrieval,
  collectUnboundRetrievalBindings,
  bindUnboundModel,
} from './template-retrieval-binding';
import { MenuItemKey, SideBar } from './template-sidebar';

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

export default function AgentTemplates() {
  const list = useFetchAgentTemplates();
  const { loading, createAgent } = useCreateAgent();
  const [searchParams] = useSearchParams();
  // canvas_category is the source tab; canvas_type(s) remain the secondary
  // theme filter rendered by the sidebar.
  const sourceCategory = resolveSourceCategory(
    searchParams.get(AgentQuery.CanvasCategory),
  );
  const [templateList, setTemplateList] = useState<IFlowTemplate[]>([]);
  const [selectMenuItem, setSelectMenuItem] = useState<string>(
    MenuItemKey.Recommended,
  );

  // The Pipeline catalog has no theme breakdown, so it renders without the
  // sidebar; the theme filter only applies to Workflow templates.
  const showSidebar = sourceCategory === AgentCategory.AgentCanvas;

  useEffect(() => {
    setTemplateList(list);
  }, [list]);

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

  const { navigateToAgent } = useNavigatePage();

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
  const handleSiderBarChange = (keyword: string) => {
    setSelectMenuItem(keyword);
  };

  const tempListFilter = useMemo(() => {
    const scoped = templateList.filter(
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
  }, [selectMenuItem, templateList, sourceCategory, showSidebar]);

  return (
    <section>
      <div className="flex flex-1 h-dvh">
        {showSidebar && (
          <SideBar
            change={handleSiderBarChange}
            selected={selectMenuItem}
          ></SideBar>
        )}

        <main className="flex-1 bg-text-title-invert/50 h-dvh">
          <CardContainer className="max-h-[94vh] overflow-auto px-8 pt-8">
            {tempListFilter?.map((x) => {
              return (
                <TemplateCard
                  key={x.id}
                  data={x}
                  showModal={showModal}
                ></TemplateCard>
              );
            })}
          </CardContainer>
          {creatingVisible && (
            <CreateAgentDialog
              loading={loading}
              visible={creatingVisible}
              hideModal={hideCreatingModal}
              selectedType={
                CanvasCategoryToFlowType[
                  template
                    ? normalizeTemplateCategory(template)
                    : sourceCategory
                ]
              }
              template={template}
              onOk={handleOk}
            ></CreateAgentDialog>
          )}
        </main>
      </div>
    </section>
  );
}
