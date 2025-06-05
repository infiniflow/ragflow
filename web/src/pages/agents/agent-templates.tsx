import { PageHeader } from '@/components/page-header';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchAgentTemplates, useSetAgent } from '@/hooks/use-agent-request';
import { IFlowTemplate } from '@/interfaces/database/flow';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateAgentDialog } from './create-agent-dialog';
import { TemplateCard } from './template-card';

export default function AgentTemplates() {
  const { navigateToAgentList } = useNavigatePage();
  const { t } = useTranslation();
  const list = useFetchAgentTemplates();
  const { loading, setAgent } = useSetAgent();

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
      let dsl = template?.dsl;
      const ret = await setAgent({
        title: payload.name,
        dsl,
        avatar: template?.avatar,
      });

      if (ret?.code === 0) {
        hideCreatingModal();
        navigateToAgent(ret.data.id)();
      }
    },
    [
      hideCreatingModal,
      navigateToAgent,
      setAgent,
      template?.avatar,
      template?.dsl,
    ],
  );

  return (
    <section>
      <PageHeader
        back={navigateToAgentList}
        title={t('flow.createGraph')}
      ></PageHeader>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-4 xl:grid-cols-6 2xl:grid-cols-8 max-h-[94vh] overflow-auto px-8">
        {list?.map((x) => {
          return (
            <TemplateCard
              key={x.id}
              data={x}
              showModal={showModal}
            ></TemplateCard>
          );
        })}
      </div>
      {creatingVisible && (
        <CreateAgentDialog
          loading={loading}
          visible={creatingVisible}
          hideModal={hideCreatingModal}
          onOk={handleOk}
        ></CreateAgentDialog>
      )}
    </section>
  );
}
