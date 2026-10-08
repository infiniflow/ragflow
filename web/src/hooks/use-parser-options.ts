import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import {
  useFetchAgentList,
  useFetchBuiltinPipelines,
} from '@/hooks/use-agent-request';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';

export type ParserOptionKind = 'builtin' | 'pipeline';

export interface IParserOption {
  value: string;
  label: string;
  kind: ParserOptionKind;
}

export const BUILTIN_PREFIX = 'builtin:';
export const PIPELINE_PREFIX = 'pipeline:';

// buildParserOptionValue packs the parser kind and the raw backend id into a
// single select value. Prefixing avoids collisions between short builtin ids
// (e.g. "general") and 32-char canvas pipeline ids, and keeps the value safe
// to serialize.
export function buildParserOptionValue(kind: ParserOptionKind, rawId: string) {
  return `${kind === 'builtin' ? BUILTIN_PREFIX : PIPELINE_PREFIX}${rawId}`;
}

export function parseParserOptionValue(value?: string): {
  kind: ParserOptionKind;
  rawId: string;
} | null {
  if (!value) return null;
  if (value.startsWith(BUILTIN_PREFIX)) {
    return { kind: 'builtin', rawId: value.slice(BUILTIN_PREFIX.length) };
  }
  if (value.startsWith(PIPELINE_PREFIX)) {
    return { kind: 'pipeline', rawId: value.slice(PIPELINE_PREFIX.length) };
  }
  return null;
}

// useParserOptions returns the unified list of parser options shown in the
// merged dataset/document parser dropdown: pipeline (canvas) options first,
// followed by builtin options each suffixed with the " (built in)" marker.
export function useParserOptions() {
  const { t } = useTranslation('knowledgeConfiguration');
  const { options: builtinOptions, loading: builtinLoading } =
    useFetchBuiltinPipelines();
  const { data: pipelineData, loading: pipelineLoading } = useFetchAgentList({
    canvas_category: AgentCategory.DataflowCanvas,
  });

  const suffix = t('builtInSuffix') || ' (built in)';

  const options = useMemo<IParserOption[]>(() => {
    const pipeline = (pipelineData?.canvas ?? [])
      .filter(
        (item) => item.type !== AgentListItemType.CompilationTemplateGroup,
      )
      .map((item) => ({
        value: buildParserOptionValue('pipeline', item.id),
        label: item.title,
        kind: 'pipeline' as const,
      }));

    const builtin = builtinOptions.map((o) => ({
      value: buildParserOptionValue('builtin', o.value),
      label: `${o.label}${suffix}`,
      kind: 'builtin' as const,
    }));

    // Pipeline options keep their existing order; builtin options are appended
    // below, matching the desired "pipeline first, builtin (built in) below" UI.
    return [...pipeline, ...builtin];
  }, [pipelineData, builtinOptions, suffix]);

  return { options, loading: builtinLoading || pipelineLoading };
}
