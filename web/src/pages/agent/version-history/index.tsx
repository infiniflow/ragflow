import { AgentBackground } from '@/components/canvas/background';
import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { PageFrame } from '@/components/page-frame';
import Spotlight from '@/components/spotlight';
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
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { SearchInput } from '@/components/ui/input';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { Spin } from '@/components/ui/spin';
import { RAGFlowTooltip } from '@/components/ui/tooltip';
import { AgentCategory } from '@/constants/agent';
import { useClientPagination } from '@/hooks/logic-hooks/use-pagination';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useApplyVersion,
  useDeleteVersion,
  useFetchVersion,
  useFetchVersionList,
  useUpdateVersion,
} from '@/hooks/use-agent-request';
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
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { AliasFormDialog } from '../components/alias-form-dialog';
import { nodeTypes } from '../canvas';
import { DropdownProvider } from '../canvas/context';
import { CanvasSnapshot, CanvasSnapshotContext } from '../context';
import { useIsPipeline } from '../hooks/use-is-pipeline';
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

type EditingAlias = { versionId: string; alias: string } | null;

export default function AgentVersionHistory() {
  const { t } = useTranslation();
  const { navigateToAgent } = useNavigatePage();
  const { id: canvasId } = useParams();
  const isPipeline = useIsPipeline();
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

  // Every version row is listed, including unpublished snapshots saved before
  // draft saves stopped writing user_canvas_version; published rows carry a
  // Dot marker next to the title.
  // A2/F1: alias filtering is a pure front-end filter over the returned set.
  const filtered = useMemo(() => {
    const q = aliasFilter.trim().toLowerCase();
    if (!q) return data;
    return data.filter((x) => (x.alias ?? '').toLowerCase().includes(q));
  }, [data, aliasFilter]);

  const { page, pageSize, setPage, onPaginationChange, pagedList } =
    useClientPagination(filtered);

  // The version detail (IFlow) has no alias field, so the preview header
  // reads the alias from the list row for the selected version.
  const selectedAlias = useMemo(
    () => data.find((x) => x.id === selectedId)?.alias,
    [data, selectedId],
  );

  // Snapshot handed to the preview so variable labels inside node components
  // resolve against the version's own graph instead of the live canvas store
  // (which holds the current draft).
  const previewSnapshot = useMemo<CanvasSnapshot | null>(() => {
    const dsl = agent?.dsl;
    if (!dsl?.graph?.nodes) {
      return null;
    }
    return {
      nodes: dsl.graph.nodes,
      edges: dsl.graph.edges ?? [],
      globals: dsl.globals ?? {},
      variables: dsl.variables ?? {},
    };
  }, [agent?.dsl]);

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

  const requestRestore = useCallback(() => {
    setPending({
      kind: 'restore',
      versionId: selectedId,
      title: agent?.title ?? '',
    });
  }, [selectedId, agent?.title]);

  const requestDelete = useCallback(() => {
    setPending({
      kind: 'delete',
      versionId: selectedId,
      title: agent?.title ?? '',
    });
  }, [selectedId, agent?.title]);

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
      if (ok) {
        navigateToAgent(
          canvasId,
          isPipeline ? AgentCategory.DataflowCanvas : AgentCategory.AgentCanvas,
        )();
      }
    }
    setPending(null);
  }, [
    pending,
    canvasId,
    deleteVersion,
    selectedId,
    applyVersion,
    navigateToAgent,
    isPipeline,
  ]);

  // Keep the current selection across list refreshes; only fall back to the
  // newest version when the selected one was pruned server-side.
  useEffect(() => {
    if (data.length > 0 && !data.some((x) => x.id === selectedId)) {
      setSelectedId(data[0].id);
    }
  }, [data, selectedId]);

  return (
    <DropdownProvider>
      <PageFrame className="p-5">
        <header className="shrink-0 pb-5">
          <h1 className="text-xl font-semibold text-text-primary">
            {t('flow.historyVersion')}
          </h1>
        </header>
        <section className="relative flex min-h-0 min-w-0 flex-1 gap-8">
          <div className="flex min-h-0 w-72 shrink-0 flex-col">
            <SearchInput
              rootClassName="m-0.5 mb-3"
              placeholder={t('flow.version.filterByAlias')}
              value={aliasFilter}
              onChange={handleAliasFilterChange}
            />
            <div className="relative min-h-0 flex-1 overflow-y-auto">
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
                          {x.release && <Dot></Dot>}
                          {x.alias && (
                            <span className="shrink-0 max-w-[20%] truncate text-accent-primary text-xs">
                              {x.alias}
                            </span>
                          )}
                          {/* Alias edit entry (design G2, section 4.4): PATCHes
  the version metadata without creating a new version or touching the DSL.
  Pencil edits an existing alias, SavePlus adds one. */}
                          <RAGFlowTooltip
                            asChild
                            tooltip={
                              x.alias
                                ? t('flow.version.editAlias')
                                : t('flow.version.addAlias')
                            }
                          >
                            <Button
                              variant="static"
                              size="auto"
                              className="ml-auto opacity-0 group-hover:opacity-100"
                              onClick={handleEditAlias(x.id, x.alias ?? '')}
                            >
                              {x.alias ? (
                                <Pencil className="size-3.5" />
                              ) : (
                                <SavePlus className="size-4" />
                              )}
                            </Button>
                          </RAGFlowTooltip>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          </div>
          <div className="relative min-h-0 min-w-0 flex-1">
            {versionLoading ? (
              <Spin className="top-1/2" />
            ) : versionLoadFailed ? (
              <div className="h-full flex items-center justify-center text-sm text-text-secondary">
                {t('flow.version.loadFailed')}
              </div>
            ) : (
              <Card className="h-full">
                <CardContent className="flex h-full min-h-0 flex-col p-5">
                  <section className="flex justify-between gap-2 pb-2">
                    <div className="min-w-0">
                      <div className="flex min-w-0 items-center">
                        <span className="pb-1 truncate">{agent?.title}</span>
                        {selectedAlias && (
                          <RAGFlowTooltip asChild tooltip={selectedAlias}>
                            <span className="ml-2 max-w-[30%] truncate text-accent-primary text-sm">
                              {selectedAlias}
                            </span>
                          </RAGFlowTooltip>
                        )}
                      </div>
                      <p className="text-text-secondary text-xs">
                        {t('flow.version.created')}:{' '}
                        {formatDate(agent?.create_date)}
                      </p>
                    </div>

                    <div className="flex items-center gap-2">
                      <RAGFlowTooltip
                        asChild
                        tooltip={t('flow.version.download')}
                      >
                        <Button
                          variant={'ghost'}
                          aria-label={t('flow.version.download')}
                          onClick={downloadFile}
                          disabled={!selectedId}
                        >
                          <ArrowDownToLine />
                        </Button>
                      </RAGFlowTooltip>
                      <RAGFlowTooltip
                        asChild
                        tooltip={t('flow.version.restore')}
                      >
                        <Button
                          variant={'ghost'}
                          aria-label={t('flow.version.restore')}
                          onClick={requestRestore}
                          disabled={!selectedId || applyRestoring}
                        >
                          <RotateCcw />
                        </Button>
                      </RAGFlowTooltip>
                      <RAGFlowTooltip
                        asChild
                        tooltip={t('flow.version.delete')}
                      >
                        <Button
                          variant={'ghost'}
                          aria-label={t('flow.version.delete')}
                          onClick={requestDelete}
                          disabled={!selectedId || deleting}
                        >
                          <Trash2 />
                        </Button>
                      </RAGFlowTooltip>
                    </div>
                  </section>
                  <section className="relative min-h-0 flex-1">
                    <ReactFlowProvider key={'flow-' + selectedId}>
                      <CanvasSnapshotContext.Provider value={previewSnapshot}>
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
                          <Spotlight
                            className="z-0"
                            opcity={0.7}
                            coverage={70}
                          />
                        </ReactFlow>
                      </CanvasSnapshotContext.Provider>
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
      </PageFrame>

      <ConfirmDeleteDialog
        open={pending?.kind === 'delete'}
        onOpenChange={handlePendingOpenChange}
        onOk={runPending}
        title={t('flow.version.deleteConfirmTitle')}
        // Pipeline versions can be pinned by datasets/files, so deleting one
        // breaks parsing; agent (workflow) versions are only historical DSL
        // snapshots, where the real risk is rolling back what embedded sites
        // and external callers are serving.
        content={{
          node: t(
            isPipeline
              ? 'flow.version.deleteConfirmContent'
              : 'flow.version.deleteAgentConfirmContent',
          ),
        }}
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
    </DropdownProvider>
  );
}
