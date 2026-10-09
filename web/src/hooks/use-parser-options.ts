import { AgentCategory } from '@/constants/agent';
import { AgentListItemType } from '@/interfaces/database/agent';
import {
  useFetchAgentList,
  useFetchBuiltinPipelines,
} from '@/hooks/use-agent-request';
import { useMemo } from 'react';

export enum ParserOptionKind {
  BuiltIn = 'builtin',
  Pipeline = 'pipeline',
}

export interface IParserOption {
  value: string;
  label: string;
  kind: ParserOptionKind;
}

// buildParserOptionValue packs the parser kind and the raw backend id into a
// single select value. Prefixing avoids collisions between short builtin ids
// (e.g. "general") and 32-char canvas pipeline ids, and keeps the value safe
// to serialize.
export function buildParserOptionValue(kind: ParserOptionKind, rawId: string) {
  return `${kind}:${rawId}`;
}

export function parseParserOptionValue(value?: string): {
  kind: ParserOptionKind;
  rawId: string;
} | null {
  if (!value) return null;
  for (const kind of Object.values(ParserOptionKind)) {
    const prefix = `${kind}:`;
    if (value.startsWith(prefix)) {
      return { kind, rawId: value.slice(prefix.length) };
    }
  }
  return null;
}

// useParserOptions returns the unified list of parser options shown in the
// merged dataset/document parser dropdown: pipeline (canvas) options first,
// followed by builtin options. Builtin options carry their plain label; the
// "built in" marker is rendered as a tag by the select component via `kind`.
export function useParserOptions() {
  const { options: builtinOptions, loading: builtinLoading } =
    useFetchBuiltinPipelines();
  const { data: pipelineData, loading: pipelineLoading } = useFetchAgentList({
    canvas_category: AgentCategory.DataflowCanvas,
  });

  const options = useMemo<IParserOption[]>(() => {
    const pipeline = (pipelineData?.canvas ?? [])
      .filter(
        (item) => item.type !== AgentListItemType.CompilationTemplateGroup,
      )
      .map((item) => ({
        value: buildParserOptionValue(ParserOptionKind.Pipeline, item.id),
        label: item.title,
        kind: ParserOptionKind.Pipeline,
      }));

    const builtin = builtinOptions.map((o) => ({
      value: buildParserOptionValue(ParserOptionKind.BuiltIn, o.value),
      label: o.label,
      kind: ParserOptionKind.BuiltIn,
    }));

    // Pipeline options keep their existing order; builtin options are appended
    // below, matching the desired "pipeline first, builtin below" UI.
    return [...pipeline, ...builtin];
  }, [pipelineData, builtinOptions]);

  return { options, loading: builtinLoading || pipelineLoading };
}
