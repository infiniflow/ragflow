import { CopyToClipboardWithText } from '@/components/copy-to-clipboard';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useFetchWebhookTrace } from '@/hooks/use-agent-request';
import { MessageEventType } from '@/hooks/use-send-message';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { upperFirst } from 'lodash';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { BeginId } from '../constant';
import { JsonViewer } from '../form/components/json-viewer';
import { WorkFlowTimeline } from './timeline';

type RunSheetProps = IModalProps<any>;

enum WebhookTraceTabType {
  Detail = 'detail',
  Tracing = 'tracing',
}

const WebhookSheet = ({ hideModal }: RunSheetProps) => {
  const { t } = useTranslation();
  const { id } = useParams();
  const text = `${location.protocol}//${location.host}/api/v1/webhook_test/${id}`;

  const { data } = useFetchWebhookTrace(true);

  const firstInput = data?.events?.find(
    (event) =>
      event.event === MessageEventType.NodeFinished &&
      event.data.component_id === BeginId,
  )?.data.inputs;

  const latestOutput = data?.events?.findLast(
    (event) =>
      event.event === MessageEventType.NodeFinished &&
      event.data.component_id !== BeginId,
  )?.data.outputs;

  const statusInfo = useMemo(() => {
    if (data?.finished === false) {
      return { status: 'running' };
    }

    let errorItem = data?.events.find(
      (x) => x.event === 'error' || x.data?.error,
    );
    if (errorItem) {
      return {
        status: 'fail',
        message: errorItem.data?.error || errorItem.message,
      };
    }
    return { status: 'success' };
  }, [data?.events, data?.finished]);

  return (
    <Sheet onOpenChange={hideModal} open modal={false}>
      <SheetContent className={cn('top-20 p-2 space-y-5 flex flex-col pb-20')}>
        <SheetHeader>
          <SheetTitle>{t('flow.testRun')}</SheetTitle>
        </SheetHeader>

        <div className="space-y-2">
          <div className="text-sm font-medium">Webhook URL:</div>
          <CopyToClipboardWithText text={text}></CopyToClipboardWithText>
        </div>

        <section>
          <span>{t('flow.webhook.agentStatus')}</span>
          <div
            className={cn({
              'text-state-error': statusInfo.status === 'fail',
              'text-state-success': statusInfo.status === 'success',
            })}
          >
            {upperFirst(statusInfo.status)}
          </div>
          <div>{statusInfo?.message}</div>
        </section>

        <Tabs
          defaultValue={WebhookTraceTabType.Detail}
          className="flex-1  min-h-0 flex flex-col"
        >
          <TabsList className="w-fit">
            <TabsTrigger value={WebhookTraceTabType.Detail}>
              {t('flow.webhook.overview')}
            </TabsTrigger>
            <TabsTrigger value={WebhookTraceTabType.Tracing}>
              {t('flow.webhook.logs')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value={WebhookTraceTabType.Detail}>
            <JsonViewer
              data={firstInput || {}}
              title={t('flow.input')}
            ></JsonViewer>
            <JsonViewer
              data={latestOutput || {}}
              title={t('flow.output')}
            ></JsonViewer>
          </TabsContent>
          <TabsContent
            value={WebhookTraceTabType.Tracing}
            className="overflow-auto flex-1"
          >
            <WorkFlowTimeline
              currentEventListWithoutMessage={data?.events || []}
              canvasId={id}
              currentMessageId=""
              sendLoading={false}
            ></WorkFlowTimeline>
          </TabsContent>
        </Tabs>
      </SheetContent>
    </Sheet>
  );
};

export default WebhookSheet;
