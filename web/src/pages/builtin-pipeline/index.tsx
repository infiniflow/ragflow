import { PageHeader } from '@/components/page-header';
import { TruncatedText } from '@/components/truncated-text';
import { Badge } from '@/components/ui/badge';
import { Spin } from '@/components/ui/spin';
import { useFetchBuiltinPipelines } from '@/hooks/use-agent-request';
import { ReactFlowProvider } from '@xyflow/react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import AgentCanvas from '../agent/canvas';
import { DropdownProvider } from '../agent/canvas/context';
import { useFetchBuiltinPipelineDataOnMount } from './use-fetch-builtin-pipeline-data';

// Built-in pipelines use the shared read-only canvas without save/run/log
// hooks. They have no secondary navigation or sidebar.
export default function BuiltinPipelinePage() {
  const { id } = useParams();
  const { t } = useTranslation();
  const { loading } = useFetchBuiltinPipelineDataOnMount();
  const { data } = useFetchBuiltinPipelines();
  const title = data.canvas.find((x) => x.id === id)?.title ?? '';

  return (
    <section className="h-full" data-testid="builtin-pipeline-page">
      <PageHeader className="pl-0">
        <section className="flex min-w-0 items-center gap-2">
          <TruncatedText
            as="h3"
            className="min-w-0 truncate text-lg font-semibold text-text-primary"
            tooltip={title}
          >
            {title}
          </TruncatedText>
          <Badge variant="secondary" className="shrink-0">
            {t('knowledgeConfiguration.builtInBadge')}
          </Badge>
        </section>
      </PageHeader>
      {loading ? (
        <div className="flex h-[calc(100%-64px)] items-center justify-center">
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
