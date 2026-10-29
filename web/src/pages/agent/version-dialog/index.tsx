import { AgentBackground } from '@/components/canvas/background';
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SearchInput } from '@/components/ui/input';
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
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
import {
  ArrowDownToLine,
  Pencil,
  RotateCcw,
  SavePlus,
  Trash2,
} from 'lucide-react';
import {
  ChangeEvent,
  MouseEvent,
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { nodeTypes } from '../canvas';
import { AliasFormDialog } from '../components/alias-form-dialog';
import { downloadDsl } from '../utils/download-dsl';

type PendingAction =
  | { kind: 'delete'; versionId: string; title: string }
  | { kind: 'restore'; versionId: string; title: string }
  | null;

type EditingAlias = { versionId: string; alias: string } | null;

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
  const [editingAlias, setEditingAlias] = useState<EditingAlias>(null);

  const { deleteVersion, loading: deleting } = useDeleteVersion();
  const { applyVersion, loading: applyRestoring } = useApplyVersion();
  const { updateVersion } = useUpdateVersion();

  // Only published versions are listed: draft saves (manual Save and
  // auto-save) no longer write user_canvas_version rows, so this dialog shows
  // what was published. Rows predating that change are filtered out here.
  const released = useMemo(() => data.filter((x) => x.release), [data]);

  // A2/F1: alias filtering is a pure front-end filter over the returned set.
  const filtered = useMemo(() => {
    const q = aliasFilter.trim().toLowerCase();
    if (!q) return released;
    return released.filter((x) => (x.alias ?? '').toLowerCase().includes(q));
  }, [released, aliasFilter]);

  const { page, pageSize, setPage, onPaginationChange, pagedList } =
    useClientPagination(filtered);

  // The version detail (IFlow) has no alias field, so the preview header
  // reads the alias from the list row for the selected version.
  const selectedAlias = useMemo(
    () => released.find((x) => x.id === selectedId)?.alias,
    [released, selectedId],
  );

  // Filtering shrinks the list, so jump back to page 1; otherwise a stale
  // page 2+ selection would render an empty list with matches out of view.
  const handleAliasFilterChange = useCallback(
    (e: ChangeEvent<HTMLInputElement>) => {
      setAliasFilter(e.target.value);
      setPage(1);
    },
    [setPage],
  );

  const handleClick = useCallback(
    (id: string) => () => {
      setSelectedId(id);
    },
    [],
  );

  const handleEditAlias = useCallback(
    (versionId: string, alias: string) => (e: MouseEvent) => {
      // Keep the icon click from also selecting the row.
      e.stopPropagation();
      setEditingAlias({ versionId, alias });
    },
    [],
  );

  const handleAliasDialogOpenChange = useCallback((open: boolean) => {
    if (!open) setEditingAlias(null);
  }, []);

  const handlePendingOpenChange = useCallback((open: boolean) => {
    if (!open) setPending(null);
  }, []);

  const handleSubmitAlias = useCallback(
    (alias?: string) => {
      if (!editingAlias || !canvasId) return;
      // An empty alias clears it (design G2/§4.4); the hook invalidates the
      // version-list cache so the row and preview header refresh in place.
      updateVersion({
        canvasId,
        versionId: editingAlias.versionId,
        alias: alias ?? '',
      });
    },
    [editingAlias, canvasId, updateVersion],
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
    if (released.length > 0 && !released.some((x) => x.id === selectedId)) {
      setSelectedId(released[0].id);
    }
  }, [released, selectedId]);

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
            <SearchInput
              rootClassName="m-0.5 mb-3"
              placeholder={t('flow.version.filterByAlias')}
              value={aliasFilter}
              onChange={handleAliasFilterChange}
            />
            {loading ? (
              <Spin className="top-1/2"></Spin>
            ) : (
              <ul className="space-y-4 text-sm">
                {pagedList.map((x) => {
                  return (
                    <li
                      key={x.id}
                      className={cn('group cursor-pointer p-2', {
                        'bg-bg-card rounded-md ': x.id === selectedId,
                      })}
                      onClick={handleClick(x.id)}
                    >
                      <div className="flex items-center gap-2">
                        <span className="truncate min-w-0">{x.title}</span>
                        {x.alias && (
                          <span className="shrink-0 max-w-[20%] truncate text-accent-primary text-xs">
                            {x.alias}
                          </span>
                        )}
                        {/* Alias edit entry (design G2, section 4.4): PATCHes
  the version metadata without creating a new version or touching the DSL.
  Pencil edits an existing alias, SavePlus adds one. */}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <button
                              type="button"
                              className="ml-auto shrink-0 text-text-secondary opacity-0 group-hover:opacity-100 hover:text-text-primary"
                              onClick={handleEditAlias(x.id, x.alias ?? '')}
                            >
                              {x.alias ? (
                                <Pencil className="size-3.5" />
                              ) : (
                                <SavePlus className="size-4" />
                              )}
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>
                            {x.alias
                              ? t('flow.version.editAlias')
                              : t('flow.version.addAlias')}
                          </TooltipContent>
                        </Tooltip>
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
                        {selectedAlias && (
                          <span className="ml-2 shrink-0 text-accent-primary text-sm">
                            {selectedAlias}
                          </span>
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
                        disabled={!selectedId || applyRestoring}
                      >
                        <RotateCcw />
                      </Button>
                      <Button
                        variant={'ghost'}
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

      <ConfirmDeleteDialog
        open={pending?.kind === 'delete'}
        onOpenChange={handlePendingOpenChange}
        onOk={runPending}
        title={t('flow.version.deleteConfirmTitle')}
        content={{ node: t('flow.version.deleteConfirmContent') }}
      />

      <AlertDialog
        open={pending?.kind === 'restore'}
        onOpenChange={handlePendingOpenChange}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('flow.version.restoreConfirmTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('flow.version.restoreConfirmContent')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={runPending}>
              {t('flow.version.restore')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {editingAlias && (
        <AliasFormDialog
          // Remount per target version so defaultValues picks up its alias.
          key={editingAlias.versionId}
          open
          onOpenChange={handleAliasDialogOpenChange}
          title={t('flow.version.editAlias')}
          description={t('flow.version.editAliasDescription')}
          initialAlias={editingAlias.alias}
          onSubmit={handleSubmitAlias}
        />
      )}
    </Dialog>
  );
}
