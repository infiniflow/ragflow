import { PageHeader } from '@/components/page-header';
import { Badge } from '@/components/ui/badge';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Spin } from '@/components/ui/spin';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchBuiltinPipelines } from '@/hooks/use-agent-request';
import { ReactFlowProvider } from '@xyflow/react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import AgentCanvas from '../agent/canvas';
import { DropdownProvider } from '../agent/canvas/context';
import { useFetchBuiltinPipelineDataOnMount } from './use-fetch-builtin-pipeline-data';

// BuiltinPipelinePage renders a built-in ingestion pipeline on the shared
// agent canvas in read-only mode: no editing, no autosave, no run/publish
// chrome. Unlike the editing page it mounts none of the save/run/log hooks
// (useWatchAgentChange, useAgentHistoryManager, pipeline log polling), so a
// built-in template can never be persisted back as a user canvas from here.
// The title comes from the pipeline catalog query rather than route state so
// a directly opened or refreshed URL still resolves it.
export default function BuiltinPipelinePage() {
  const { id } = useParams();
  const { t } = useTranslation();
  const { navigateToAgents } = useNavigatePage();
  const { loading } = useFetchBuiltinPipelineDataOnMount();
  const { data } = useFetchBuiltinPipelines();
  const title = data.canvas.find((x) => x.id === id)?.title ?? '';

  return (
    <section className="h-full" data-testid="builtin-pipeline-page">
      <PageHeader>
        <section>
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink onClick={navigateToAgents}>
                  {t('flow.agents')}
                </BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage className="flex items-center gap-2">
                  {title}
                  <Badge variant="secondary">
                    {t('knowledgeConfiguration.builtInBadge')}
                  </Badge>
                </BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </section>
      </PageHeader>
      {loading ? (
        <div className="flex items-center justify-center h-[calc(100%-64px)]">
          <Spin size="large" />
        </div>
      ) : (
        <ReactFlowProvider>
          <DropdownProvider>
            <AgentCanvas readOnly />
          </DropdownProvider>
        </ReactFlowProvider>
      )}
    </section>
  );
}
