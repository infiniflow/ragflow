import { AgentBackground } from '@/components/canvas/background';
import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import { useClientPagination } from '@/hooks/logic-hooks/use-pagination';
import {
  useFetchVersion,
  useFetchVersionList,
} from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { downloadJsonFile } from '@/utils/file-util';
import { ConnectionMode, ReactFlow, ReactFlowProvider } from '@xyflow/react';
import { ArrowDownToLine } from 'lucide-react';
import { ReactNode, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { nodeTypes } from '../canvas';

export function VersionDialog({
  hideModal,
}: IModalProps<any> & { initialName?: string; title?: ReactNode }) {
  const { t } = useTranslation();
  const { data, loading } = useFetchVersionList();
  const [selectedId, setSelectedId] = useState<string>('');
  const { data: agent, loading: versionLoading } = useFetchVersion(selectedId);

  const { page, pageSize, onPaginationChange, pagedList } =
    useClientPagination(data);

  const handleClick = useCallback(
    (id: string) => () => {
      setSelectedId(id);
    },
    [],
  );

  const downloadFile = useCallback(() => {
    const graph = agent?.dsl.graph;
    if (graph) {
      downloadJsonFile(graph, agent?.title);
    }
  }, [agent?.dsl.graph, agent?.title]);

  useEffect(() => {
    if (data.length > 0) {
      setSelectedId(data[0].id);
    }
  }, [data]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-w-[60vw]">
        <DialogHeader>
          <DialogTitle className="text-base">
            {t('flow.historyversion')}
          </DialogTitle>
        </DialogHeader>
        <section className="flex gap-8 relative">
          <div className="w-1/3 max-h-[60vh] overflow-auto min-h-[40vh]">
            {loading ? (
              <Spin className="top-1/2"></Spin>
            ) : (
              <ul className="space-y-4 text-sm">
                {pagedList.map((x) => (
                  <li
                    key={x.id}
                    className={cn('cursor-pointer p-2', {
                      'bg-card rounded-md ': x.id === selectedId,
                    })}
                    onClick={handleClick(x.id)}
                  >
                    {x.title}
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div className="relative flex-1 ">
            {versionLoading ? (
              <Spin className="top-1/2" />
            ) : (
              <Card className="h-full">
                <CardContent className="h-full p-5 flex flex-col">
                  <section className="flex justify-between">
                    <div>
                      <div className="pb-1 truncate">{agent?.title}</div>
                      <p className="text-text-secondary text-xs">
                        Created: {formatDate(agent?.create_date)}
                      </p>
                    </div>
                    <Button variant={'ghost'} onClick={downloadFile}>
                      <ArrowDownToLine />
                    </Button>
                  </section>
                  <section className="relative flex-1">
                    <ReactFlowProvider key={`flow-${selectedId}`}>
                      <ReactFlow
                        connectionMode={ConnectionMode.Loose}
                        nodes={agent?.dsl.graph?.nodes || []}
                        edges={
                          agent?.dsl.graph?.edges.flatMap((x) => ({
                            ...x,
                            type: 'default',
                          })) || []
                        }
                        fitView
                        nodeTypes={nodeTypes}
                        edgeTypes={{}}
                        zoomOnScroll={true}
                        panOnDrag={true}
                        zoomOnDoubleClick={false}
                        preventScrolling={true}
                        minZoom={0.1}
                      >
                        <AgentBackground></AgentBackground>
                        <Spotlight className="z-0" opcity={0.7} coverage={70} />
                      </ReactFlow>
                    </ReactFlowProvider>
                  </section>
                </CardContent>
              </Card>
            )}
          </div>
        </section>
        <RAGFlowPagination
          total={data.length}
          current={page}
          pageSize={pageSize}
          onChange={onPaginationChange}
        ></RAGFlowPagination>
      </DialogContent>
    </Dialog>
  );
}
