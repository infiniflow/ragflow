import { AgentCategory } from '@/constants/agent';
import {
  ICanvasVersionRow,
  useFetchAgentList,
  useFetchBuiltinPipelines,
  useFetchCanvasVersions,
} from '@/hooks/use-agent-request';
import {
  buildParserOptionValue,
  parseParserOptionValue,
  ParserOptionKind,
} from '@/hooks/use-parser-options';
import { AgentListItemType } from '@/interfaces/database/agent';
import { Loader2 } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  filterTreeNodes,
  TreeSelectNode,
  withBranchIds,
} from '../tree-select-utils';

// Node id of a pipeline branch (the pipeline draft itself). Branches are never
// selectable in TreeSelect — only their version leaves are — so this id is
// never emitted as a value.
const buildPipelineNodeId = (pipelineId: string) =>
  buildParserOptionValue(ParserOptionKind.Pipeline, pipelineId);

// useParserTreeData assembles the merged parser tree: dataflow pipelines as
// branches with their published versions as leaves, builtin parsers as
// root-level leaves. Versions are fetched lazily — for the currently selected
// pipeline (needed for the closed-select display) and for every branch the
// user has expanded.
export function useParserTreeData({ value }: { value?: string }) {
  const { t } = useTranslation();
  const { options: builtinOptions, loading: builtinLoading } =
    useFetchBuiltinPipelines();
  const { data: pipelineData, loading: pipelineLoading } = useFetchAgentList({
    canvas_category: AgentCategory.DataflowCanvas,
  });

  const selectedPipelineId = useMemo(() => {
    const parsed = parseParserOptionValue(value);
    return parsed?.kind === ParserOptionKind.Pipeline
      ? parsed.rawId
      : undefined;
  }, [value]);

  // Expansion is controlled here (not inside TreeSelect) so that expansion
  // drives the lazy version fetch below.
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  // Render-phase adjustment (React's endorsed "adjust state when a prop
  // changes" pattern): the branch holding the selection starts expanded. The
  // previous-id comparison keeps a user's later manual collapse intact.
  const [prevSelectedPipelineId, setPrevSelectedPipelineId] = useState<
    string | undefined
  >(undefined);
  if (selectedPipelineId !== prevSelectedPipelineId) {
    setPrevSelectedPipelineId(selectedPipelineId);
    if (selectedPipelineId) {
      const nodeId = buildPipelineNodeId(selectedPipelineId);
      setExpandedIds((prev) => {
        if (prev.has(nodeId)) return prev;
        const next = new Set(prev);
        next.add(nodeId);
        return next;
      });
    }
  }

  // Versions load for the selected pipeline plus every expanded branch. The
  // set is derived, so collapsing a branch drops its observer and a re-expand
  // refetches (gcTime: 0) — always-fresh data over a cached shortcut.
  const versionCanvasIds = useMemo(() => {
    const ids = new Set<string>();
    if (selectedPipelineId) ids.add(selectedPipelineId);
    expandedIds.forEach((nodeId) => {
      const parsed = parseParserOptionValue(nodeId);
      if (parsed?.kind === ParserOptionKind.Pipeline) ids.add(parsed.rawId);
    });
    // Sorted so the useQueries array stays stable across expansion changes.
    return [...ids].sort();
  }, [selectedPipelineId, expandedIds]);
  const versionResults = useFetchCanvasVersions(versionCanvasIds);

  const { versionsByCanvas, loadingCanvasIds } = useMemo(() => {
    const versionsByCanvas = new Map<string, ICanvasVersionRow[]>();
    const loadingCanvasIds = new Set<string>();
    versionCanvasIds.forEach((canvasId, index) => {
      const query = versionResults[index];
      if (query?.isSuccess) {
        versionsByCanvas.set(canvasId, query.data ?? []);
      } else {
        loadingCanvasIds.add(canvasId);
      }
    });
    return { versionsByCanvas, loadingCanvasIds };
  }, [versionResults, versionCanvasIds]);

  // Trim: some locales still define the marker with surrounding whitespace
  // from when it was appended to the label text.
  const builtInTag = t('knowledgeConfiguration.builtInSuffix').trim();

  const treeData = useMemo<TreeSelectNode[]>(() => {
    const pipelines = (pipelineData?.canvas ?? [])
      .filter(
        (item) => item.type !== AgentListItemType.CompilationTemplateGroup,
      )
      .map((item): TreeSelectNode => {
        const nodeId = buildPipelineNodeId(item.id);
        const versions = versionsByCanvas.get(item.id);
        let children: TreeSelectNode[];
        if (!versions) {
          children = [
            {
              id: `${nodeId}:loading`,
              title: '',
              disabled: true,
              label: (
                <Loader2 className="size-3.5 animate-spin text-text-disabled" />
              ),
            },
          ];
        } else if (versions.length === 0) {
          // A pipeline draft without published versions stays visible but
          // offers nothing to select. The disabled placeholder row also keeps
          // the branch non-empty, so TreeSelect never treats the draft itself
          // as a selectable leaf.
          children = [
            {
              id: `${nodeId}:empty`,
              title: '',
              disabled: true,
              label: (
                <span className="text-text-disabled">
                  {t('flow.version.noPublished')}
                </span>
              ),
            },
          ];
        } else {
          children = versions.map((version): TreeSelectNode => ({
            id: buildParserOptionValue(
              ParserOptionKind.Pipeline,
              item.id,
              version.id,
            ),
            // The alias joins the plain-text title so TreeSelect search
            // matches it; `label` renders it as secondary text instead.
            title: version.alias
              ? `${version.title} ${version.alias}`
              : version.title,
            searchKeywords: version.alias ? [version.alias] : undefined,
            label: (
              <span className="flex items-center gap-1.5 min-w-0">
                <span className="truncate">{version.title}</span>
                {version.alias && (
                  <span className="text-text-secondary truncate flex-shrink-0">
                    {version.alias}
                  </span>
                )}
              </span>
            ),
          }));
        }
        return { id: nodeId, title: item.title, children };
      });

    const builtins = builtinOptions.map((option): TreeSelectNode => ({
      id: buildParserOptionValue(ParserOptionKind.BuiltIn, option.value),
      title: option.label,
      searchKeywords: [option.label],
      label: (
        <span className="flex items-center gap-1.5 min-w-0">
          <span className="truncate">{option.label}</span>
          <span className="shrink-0 rounded-md bg-bg-card px-1.5 py-0.5 text-xs text-text-secondary">
            {builtInTag}
          </span>
        </span>
      ),
    }));

    // Pipeline branches first, builtin leaves below — the same ordering the
    // flat parser select used.
    return [...pipelines, ...builtins];
  }, [pipelineData, builtinOptions, versionsByCanvas, t, builtInTag]);

  // TreeSelect's built-in search auto-expansion only runs for uncontrolled
  // expansion; with expansion owned here, matching branches are expanded from
  // the search event instead. filterTreeNodes mirrors TreeSelect's keep-rule
  // exactly, and every branch surviving the filter gets expanded, matching
  // the built-in behavior.
  const handleSearchChange = useCallback(
    (term: string) => {
      if (!term.trim()) return;
      const visible = filterTreeNodes(treeData, term);
      setExpandedIds((prev) => withBranchIds(prev, visible));
    },
    [treeData],
  );

  // An unmatched value is "missing" only once everything that could contain
  // it has loaded; while the selected pipeline's versions are still arriving
  // the select must not flash the missing display.
  const loading =
    builtinLoading ||
    pipelineLoading ||
    (!!selectedPipelineId && loadingCanvasIds.has(selectedPipelineId));

  const selectedLeafExists = useMemo(() => {
    if (!value) return false;
    const find = (nodes: TreeSelectNode[]): boolean =>
      nodes.some(
        (node) =>
          node.id === value || (node.children ? find(node.children) : false),
      );
    return find(treeData);
  }, [treeData, value]);

  // Node-id keyed (not canvas-id keyed) so TreeSelect can spin the chevron of
  // a branch whose versions are being fetched.
  const loadingIds = useMemo(
    () => new Set([...loadingCanvasIds].map(buildPipelineNodeId)),
    [loadingCanvasIds],
  );

  return {
    treeData,
    expandedIds,
    onExpandedChange: setExpandedIds,
    onSearchChange: handleSearchChange,
    loadingIds,
    loading,
    selectedLeafExists,
  };
}
