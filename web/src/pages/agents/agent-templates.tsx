import { PageHeader } from '@/components/page-header';
import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlowTemplates } from '@/hooks/flow-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { CreateAgentDialog } from './create-agent-dialog';
import { TemplateCard } from './template-card';

export default function AgentTemplates() {
  const { navigateToAgentList } = useNavigatePage();
  const { t } = useTranslation();
  const { data: list } = useFetchFlowTemplates();
  const {
    visible: creatingVisible,
    hideModal: hideCreatingModal,
    showModal: showCreatingModal,
  } = useSetModalState();

  const handleOk = useCallback(async () => {
    // return onOk(name, checkedId);
  }, []);

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
              showModal={showCreatingModal}
            ></TemplateCard>
          );
        })}
      </div>
      {creatingVisible && (
        <CreateAgentDialog
          loading={false}
          visible={creatingVisible}
          hideModal={hideCreatingModal}
          onOk={handleOk}
        ></CreateAgentDialog>
      )}
    </section>
  );
}
