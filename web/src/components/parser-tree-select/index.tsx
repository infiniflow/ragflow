import { RAGFlowTooltip } from '@/components/ui/tooltip';
import {
  parseParserOptionValue,
  ParserOptionKind,
} from '@/hooks/use-parser-options';
import { cn } from '@/lib/utils';
import { AlertCircle, TriangleAlert } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { PipelineVersionLabel } from '../pipeline-version-label';
import { TreeSelect, TreeSelectNode } from '../tree-select';
import { useParserTreeData } from './use-parser-tree-data';

export interface ParserTreeSelectProps {
  // Packed value produced by buildParserSelectValue, e.g. "builtin:general" or
  // "pipeline:<pipelineId>:<versionId>".
  value?: string;
  // Called with the resolved kind, the raw backend id and (for pipeline
  // version leaves) the version id on selection.
  onChange: (
    kind: ParserOptionKind | null,
    rawId: string,
    versionId?: string,
  ) => void;
  // Shows a non-interactive warning icon at the end of the trigger when the
  // pinned version is no longer the latest. The two callers define "outdated"
  // differently (dataset settings: only while the saved pin is still held;
  // document dialog: whenever the selection differs from latest), so the
  // component only renders what the caller computes.
  outdated?: boolean;
  // True when the owning form field failed validation (e.g. the saved parser
  // option no longer exists) — paints the trigger border like other invalid
  // form controls.
  invalid?: boolean;
  placeholder?: string;
  disabled?: boolean;
}

// ParserTreeSelect is the single dropdown that merges builtin parsers and
// dataflow pipelines with their published versions: builtin parsers are
// selectable root-level leaves, pipelines are non-selectable branches (the
// draft) whose version leaves are the only pipeline-side choices. It is a
// controlled component: the parent owns the form fields and translates the
// kind/id pair into parser_id / pipeline_id / pipeline_version_id /
// parse_type (see useParserSelectHandler).
export function ParserTreeSelect({
  value,
  onChange,
  outdated,
  invalid,
  placeholder,
  disabled,
}: ParserTreeSelectProps) {
  const { t } = useTranslation();
  const {
    treeData,
    expandedIds,
    onExpandedChange,
    onSearchChange,
    loadingIds,
    loading,
    selectedLeafExists,
  } = useParserTreeData({ value });

  const handleChange = useCallback(
    (id: string) => {
      const parsed = parseParserOptionValue(id);
      if (!parsed) {
        onChange(null, '');
        return;
      }
      onChange(parsed.kind, parsed.rawId, parsed.versionId);
    },
    [onChange],
  );

  // Reuse the leaf's rich label (title + secondary alias / builtin tag) for
  // the closed-select display, except for aliased versions: there the trigger
  // caps the alias at 30% of its width with an ellipsis (the dropdown row
  // keeps the full alias). While the option list is still loading the node is
  // undefined — fall back to the placeholder instead of a blank.
  const renderSelected = useCallback(
    (node: TreeSelectNode | undefined) => {
      if (!node) return placeholder ?? t('common.pleaseSelect');
      const alias = node.data?.alias;
      if (alias) {
        return (
          <PipelineVersionLabel
            title={node.data?.title ?? ''}
            alias={alias}
            className="w-full"
            aliasClassName="max-w-[30%]"
          />
        );
      }
      return node.label ?? node.title;
    },
    [placeholder, t],
  );

  // When the saved value references a parser or version that no longer exists
  // (e.g. a deleted pipeline or pruned version) and options have finished
  // loading, render a localized "unavailable" label instead of the raw
  // internal value.
  const renderMissingParser = useCallback(
    () => (
      <span className="flex items-center gap-1.5">
        <TriangleAlert className="size-4 flex-shrink-0" />
        <span className="truncate">
          {t('knowledgeConfiguration.parserOptionUnavailable', {
            defaultValue: 'unavailable',
          })}
        </span>
      </span>
    ),
    [t],
  );

  // The outdated icon is a pure hint: the tooltip trigger swallows its own
  // click before it bubbles to the TreeSelect trigger, so clicking the icon
  // neither toggles the dropdown nor triggers the lazy version fetches that
  // follow. It is hidden while the value is already reported as missing — a
  // deleted pin is the stronger signal.
  const stopHintClick = useCallback((e: React.SyntheticEvent) => {
    e.stopPropagation();
  }, []);

  const trailingAddon =
    outdated && (loading || selectedLeafExists) ? (
      <RAGFlowTooltip
        tooltip={t('flow.version.outdated')}
        onClick={stopHintClick}
        onPointerDown={stopHintClick}
      >
        <AlertCircle className="size-4 flex-shrink-0" />
      </RAGFlowTooltip>
    ) : undefined;

  return (
    <TreeSelect
      data={treeData}
      value={value}
      onChange={handleChange}
      placeholder={
        placeholder ?? t('knowledgeConfiguration.parserSelectPlaceholder')
      }
      disabled={disabled}
      className={cn(invalid && 'border-destructive')}
      showSearch
      expandedIds={expandedIds}
      onExpandedChange={onExpandedChange}
      onSearchChange={onSearchChange}
      loadingIds={loadingIds}
      loading={loading}
      renderSelected={renderSelected}
      renderMissingValue={renderMissingParser}
      trailingAddon={trailingAddon}
    />
  );
}
