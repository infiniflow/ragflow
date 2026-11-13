import { ParseType } from '@/constants/knowledge';

export enum ParserOptionKind {
  BuiltIn = 'builtin',
  Pipeline = 'pipeline',
}

// buildParserOptionValue packs the parser kind, the raw backend id and (for
// pipelines) the pinned version id into a single select value, e.g.
// "builtin:general" or "pipeline:<pipelineId>:<versionId>". Prefixing avoids
// collisions between short builtin ids (e.g. "general") and 32-char canvas
// pipeline ids, and keeps the value safe to serialize. Pipeline and version
// ids are colon-free hex strings, so splitting on ':' is unambiguous.
export function buildParserOptionValue(
  kind: ParserOptionKind,
  rawId: string,
  versionId?: string,
) {
  return versionId ? `${kind}:${rawId}:${versionId}` : `${kind}:${rawId}`;
}

export function parseParserOptionValue(value?: string): {
  kind: ParserOptionKind;
  rawId: string;
  versionId?: string;
} | null {
  if (!value) return null;
  for (const kind of Object.values(ParserOptionKind)) {
    const prefix = `${kind}:`;
    if (value.startsWith(prefix)) {
      const rest = value.slice(prefix.length);
      if (kind === ParserOptionKind.Pipeline) {
        const separator = rest.indexOf(':');
        if (separator > 0) {
          return {
            kind,
            rawId: rest.slice(0, separator),
            versionId: rest.slice(separator + 1),
          };
        }
      }
      return { kind, rawId: rest };
    }
  }
  return null;
}

// Builtin parser ids saved by older versions, mapped to the id the current
// builtin pipeline catalog uses. The Go backend resolves the same aliases at
// lookup time (internal/ingestion/pipeline/builtin_registry.go); the select
// needs them too, otherwise a saved parser_id like "naive" matches no option
// and renders as unavailable.
const LegacyBuiltinParserAliases: Record<string, string> = {
  naive: 'general',
};

export function resolveBuiltinParserId(rawId: string) {
  return LegacyBuiltinParserAliases[rawId] ?? rawId;
}

// buildParserSelectValue derives the ParserTreeSelect value from the form's
// parse_type / parser_id / pipeline_id / pipeline_version_id fields. Builtin
// ids pass through the legacy alias map so old saved values keep matching an
// option. A pipeline without a concrete version (an unpinned binding whose
// version list has not resolved yet) yields no value, so the select shows its
// placeholder until the latest-version prefill fills the field in. An empty id
// likewise yields undefined instead of a dangling "kind:" value.
export function buildParserSelectValue({
  parseType,
  parserId,
  pipelineId,
  pipelineVersionId,
}: {
  parseType?: ParseType;
  parserId?: string;
  pipelineId?: string;
  pipelineVersionId?: string;
}): string | undefined {
  if (parseType === ParseType.BuiltIn) {
    return parserId
      ? buildParserOptionValue(
          ParserOptionKind.BuiltIn,
          resolveBuiltinParserId(parserId),
        )
      : undefined;
  }
  return pipelineId && pipelineVersionId
    ? buildParserOptionValue(
        ParserOptionKind.Pipeline,
        pipelineId,
        pipelineVersionId,
      )
    : undefined;
}
