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
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
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

  const firstInput = data?.events.find(
    (event) =>
      event.event === MessageEventType.NodeFinished &&
      event.data.component_id === BeginId,
  )?.data.inputs;

  const latestOutput = data?.events?.findLast(
    (event) =>
      event.event === MessageEventType.NodeFinished &&
      event.data.component_id !== BeginId,
  )?.data.outputs;

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
          <div className="text-state-success">
            {data?.finished ? 'SUCCESS' : 'RUNNING'}
          </div>
        </section>

        <Tabs
          defaultValue={WebhookTraceTabType.Detail}
          className="flex-1  min-h-0 flex flex-col"
        >
          <TabsList className="w-fit">
            <TabsTrigger value={WebhookTraceTabType.Detail}>Detail</TabsTrigger>
            <TabsTrigger value={WebhookTraceTabType.Tracing}>
              Tracing
            </TabsTrigger>
          </TabsList>
          <TabsContent value={WebhookTraceTabType.Detail}>
            <JsonViewer data={firstInput || {}} title={'Input'}></JsonViewer>
            <JsonViewer data={latestOutput || {}} title={'Output'}></JsonViewer>
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
