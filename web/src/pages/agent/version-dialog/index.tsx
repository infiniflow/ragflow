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
import { Input } from '@/components/ui/input';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import { AntToolTip, RAGFlowTooltip } from '@/components/ui/tooltip';
import { useClientPagination } from '@/hooks/logic-hooks/use-pagination';
import {
  useApplyVersion,
  useDeleteVersion,
  useFetchVersion,
  useFetchVersionList,
  useUpdateVersion,
} from '@/hooks/use-agent-request';
import { IModalProps } from '@/interfaces/common';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/date';
import { ConnectionMode, ReactFlow, ReactFlowProvider } from '@xyflow/react';
import { ArrowDownToLine, RotateCcw, Trash2 } from 'lucide-react';
import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { nodeTypes } from '../canvas';
import { downloadDsl } from '../utils/download-dsl';

function Dot() {
  return (
    <span className="w-2 h-2 inline-block rounded-full bg-accent-primary flex-shrink-0" />
  );
}

type PendingAction =
  | { kind: 'delete'; versionId: string; title: string }
  | { kind: 'restore'; versionId: string; title: string }
  | null;

// AliasInput is a controlled input for a single canvas version's free-text alias
// (design G2/section 4.4). It keeps the value in local state and is keyed by
// version id, so parent re-renders (including version-list refetches triggered by
// focus changes or row selection) never unmount the field and wipe in-progress
// keystrokes the way an uncontrolled defaultValue input with a volatile key could.
const AliasInput = ({
  placeholder,
  initialAlias,
  onCommit,
}: {
  placeholder: string;
  initialAlias: string;
  onCommit: (alias: string) => void;
}) => {
  const [val, setVal] = useState(initialAlias);
  return (
    <Input
      className="h-7 text-xs"
      placeholder={placeholder}
      value={val}
      onChange={(e) => setVal(e.target.value)}
      onClick={(e) => e.stopPropagation()}
      onBlur={() => {
        const v = val.trim();
        if (v === initialAlias) return;
        onCommit(v);
      }}
    />
  );
};

export function VersionDialog({
  hideModal,
}: IModalProps<any> & { initialName?: string; title?: ReactNode }) {
  const { t } = useTranslation();
  const { id: canvasId } = useParams();
  const { data, loading } = useFetchVersionList();
  const [selectedId, setSelectedId] = useState<string>('');
  const {
    data: agent,
    loading: versionLoading,
    isError: versionLoadFailed,
  } = useFetchVersion(selectedId);
  const [aliasFilter, setAliasFilter] = useState('');
  const [pending, setPending] = useState<PendingAction>(null);

  const { deleteVersion, loading: deleting } = useDeleteVersion();
  const { applyVersion, loading: applyRestoring } = useApplyVersion();
  const { updateVersion } = useUpdateVersion();

  // Backend returns the list newest-first (design section 7/E1); the first
  // entry is therefore the latest version and every other entry is an older
  // snapshot. Non-latest versions get a D1/D2/D3 `!` marker.
  const latestId = data[0]?.id;

  // A2/F1: alias filtering is a pure front-end filter over the returned set.
  const filtered = useMemo(() => {
    const q = aliasFilter.trim().toLowerCase();
    if (!q) return data;
    return data.filter((x) => (x.alias ?? '').toLowerCase().includes(q));
  }, [data, aliasFilter]);

  const { page, pageSize, onPaginationChange, pagedList } =
    useClientPagination(filtered);

  const handleClick = useCallback(
    (id: string) => () => {
      setSelectedId(id);
    },
    [],
  );

  const downloadFile = useCallback(() => {
    if (agent?.dsl) {
      downloadDsl(agent.dsl, agent.title);
    }
  }, [agent?.dsl, agent?.title]);

  const runPending = useCallback(async () => {
    if (!pending || !canvasId) return;
    if (pending.kind === 'delete') {
      // deleteVersion resolves to a success boolean (code === 0). Only drop the
      // selection on success, so a failed delete keeps the current version
      // selected for retry instead of silently clearing the preview.
      const ok = await deleteVersion({
        canvasId,
        versionId: pending.versionId,
      });
      if (ok && selectedId === pending.versionId) setSelectedId('');
    } else if (pending.kind === 'restore') {
      const ok = await applyVersion({
        canvasId,
        versionId: pending.versionId,
      });
      if (ok) hideModal?.();
    }
    setPending(null);
  }, [pending, canvasId, deleteVersion, selectedId, applyVersion, hideModal]);

  // Keep the current selection across list refreshes; only fall back to the
  // newest version when the selected one was pruned server-side.
  useEffect(() => {
    if (data.length > 0 && !data.some((x) => x.id === selectedId)) {
      setSelectedId(data[0].id);
    }
  }, [data, selectedId]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="max-w-[900px]">
        <DialogHeader>
          <DialogTitle className="text-base">
            {t('flow.historyVersion')}
          </DialogTitle>
        </DialogHeader>
        <section className="flex gap-8 relative min-w-0">
          <div className="w-72 shrink-0 max-h-[60vh] overflow-auto min-h-[40vh]">
            <Input
              className="mb-3"
              placeholder={t('flow.version.filterByAlias')}
              value={aliasFilter}
              onChange={(e) => setAliasFilter(e.target.value)}
            />
            {loading ? (
              <Spin className="top-1/2"></Spin>
            ) : (
              <ul className="space-y-4 text-sm">
                {pagedList.map((x) => {
                  const isLatest = x.id === latestId;
                  return (
                    <li
                      key={x.id}
                      className={cn('cursor-pointer p-2', {
                        'bg-card rounded-md ': x.id === selectedId,
                      })}
                      onClick={handleClick(x.id)}
                    >
                      <div className="flex items-center gap-2">
                        <span className="truncate min-w-0">{x.title}</span>
                        {x.release && <Dot></Dot>}
                        {!isLatest && (
                          <AntToolTip
                            title={t('flow.version.outdated')}
                            trigger="click"
                          >
                            <span className="text-destructive font-bold">
                              !
                            </span>
                          </AntToolTip>
                        )}
                      </div>
                      <div className="mt-1">
                        {/* Editable alias (design G2, section 4.4): PATCHes the
  version metadata without creating a new version or touching the DSL.
  Clicking the input must not also select the row. Controlled via AliasInput
  so re-renders never wipe in-progress keystrokes. */}
                        <AliasInput
                          placeholder={t('flow.version.alias')}
                          initialAlias={x.alias ?? ''}
                          onCommit={(alias) =>
                            updateVersion({
                              canvasId: canvasId ?? '',
                              versionId: x.id,
                              alias,
                            })
                          }
                        />
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
          <div className="relative flex-1 min-w-0">
            {versionLoading ? (
              <Spin className="top-1/2" />
            ) : versionLoadFailed ? (
              <div className="h-full flex items-center justify-center text-sm text-text-secondary">
                {t('flow.version.loadFailed')}
              </div>
            ) : (
              <Card className="h-full">
                <CardContent className="h-full p-5 flex flex-col">
                  <section className="flex justify-between gap-2 pb-2">
                    <div className="min-w-0">
                      <div className="flex min-w-0 items-center">
                        <span className="pb-1 truncate">{agent?.title}</span>
                        {agent?.release && (
                          <RAGFlowTooltip tooltip={t('flow.productionTooltip')}>
                            <Button className="bg-accent-primary-5 ml-3">
                              <Dot></Dot>
                              <span className="text-accent-primary pl-2 rounded">
                                {t('flow.production')}
                              </span>
                            </Button>
                          </RAGFlowTooltip>
                        )}
                        {selectedId !== latestId && (
                          <AntToolTip
                            title={t('flow.version.outdated')}
                            trigger="click"
                          >
                            <span className="ml-2 text-destructive font-bold cursor-help">
                              !
                            </span>
                          </AntToolTip>
                        )}
                      </div>
                      <p className="text-text-secondary text-xs">
                        {t('flow.version.created')}:{' '}
                        {formatDate(agent?.create_date)}
                      </p>
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        variant={'ghost'}
                        onClick={downloadFile}
                        title={t('flow.version.download')}
                      >
                        <ArrowDownToLine />
                      </Button>
                      <Button
                        variant={'ghost'}
                        onClick={() =>
                          setPending({
                            kind: 'restore',
                            versionId: selectedId,
                            title: agent?.title ?? '',
                          })
                        }
                        title={t('flow.version.restore')}
                        disabled={
                          selectedId === latestId ||
                          !selectedId ||
                          applyRestoring
                        }
                      >
                        <RotateCcw />
                      </Button>
                      <Button
                        variant={'ghost'}
                        className="text-destructive"
                        onClick={() =>
                          setPending({
                            kind: 'delete',
                            versionId: selectedId,
                            title: agent?.title ?? '',
                          })
                        }
                        title={t('flow.version.delete')}
                        disabled={!selectedId || deleting}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </section>
                  <section className="relative flex-1">
                    <ReactFlowProvider key={'flow-' + selectedId}>
                      <ReactFlow
                        connectionMode={ConnectionMode.Loose}
                        nodes={agent?.dsl?.graph?.nodes || []}
                        edges={
                          agent?.dsl?.graph?.edges?.flatMap((x) => ({
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
                        nodesDraggable={false}
                        nodesConnectable={false}
                        elementsSelectable={false}
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
          total={filtered.length}
          current={page}
          pageSize={pageSize}
          onChange={onPaginationChange}
        ></RAGFlowPagination>
      </DialogContent>

      <AlertDialog
        open={!!pending}
        onOpenChange={(open) => {
          if (!open) setPending(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {pending?.kind === 'delete'
                ? t('flow.version.deleteConfirmTitle')
                : t('flow.version.restoreConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {pending?.kind === 'delete'
                ? t('flow.version.deleteConfirmContent')
                : t('flow.version.restoreConfirmContent')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={runPending}
              className={
                pending?.kind === 'delete'
                  ? 'bg-destructive hover:bg-destructive'
                  : ''
              }
            >
              {pending?.kind === 'delete'
                ? t('flow.version.delete')
                : t('flow.version.restore')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Dialog>
  );
}
