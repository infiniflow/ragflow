import { ParseType } from '@/constants/knowledge';
import {
  buildParserOptionValue,
  buildParserSelectValue,
  parseParserOptionValue,
  ParserOptionKind,
  resolveBuiltinParserId,
} from '@/hooks/use-parser-options';

describe('parser option value encoding', () => {
  it('prefixes builtin and pipeline ids distinctly', () => {
    expect(buildParserOptionValue(ParserOptionKind.BuiltIn, 'general')).toBe(
      'builtin:general',
    );
    expect(buildParserOptionValue(ParserOptionKind.Pipeline, 'abc123')).toBe(
      'pipeline:abc123',
    );
  });

  it('packs the version id into pipeline leaf values', () => {
    expect(
      buildParserOptionValue(ParserOptionKind.Pipeline, 'abc123', 'v9'),
    ).toBe('pipeline:abc123:v9');
  });

  it('round-trips builtin and pipeline values', () => {
    expect(parseParserOptionValue('builtin:general')).toEqual({
      kind: ParserOptionKind.BuiltIn,
      rawId: 'general',
    });
    expect(parseParserOptionValue('pipeline:abc123')).toEqual({
      kind: ParserOptionKind.Pipeline,
      rawId: 'abc123',
    });
    expect(parseParserOptionValue('pipeline:abc123:v9')).toEqual({
      kind: ParserOptionKind.Pipeline,
      rawId: 'abc123',
      versionId: 'v9',
    });
  });

  it('keeps builtin and pipeline ids with the same bare value distinct', () => {
    // A short builtin id and a pipeline id that happens to share the bare value
    // must not collide once prefixed.
    const builtin = buildParserOptionValue(ParserOptionKind.BuiltIn, 'general');
    const pipeline = buildParserOptionValue(
      ParserOptionKind.Pipeline,
      'general',
    );
    expect(builtin).not.toBe(pipeline);
    expect(parseParserOptionValue(builtin)?.kind).toBe(
      ParserOptionKind.BuiltIn,
    );
    expect(parseParserOptionValue(pipeline)?.kind).toBe(
      ParserOptionKind.Pipeline,
    );
  });

  it('splits pipeline leaf values on the first colon after the prefix', () => {
    // Pipeline and version ids are colon-free hex strings, so the first ':'
    // after the prefix separates them; anything further stays in the version
    // part. "pipeline:a:b:c" therefore decodes as pipeline "a", version "b:c".
    expect(parseParserOptionValue('pipeline:a:b:c')).toEqual({
      kind: ParserOptionKind.Pipeline,
      rawId: 'a',
      versionId: 'b:c',
    });
  });

  it('returns null for empty or unprefixed values', () => {
    expect(parseParserOptionValue('')).toBeNull();
    expect(parseParserOptionValue(undefined)).toBeNull();
    expect(parseParserOptionValue('general')).toBeNull();
    expect(parseParserOptionValue('unknown:general')).toBeNull();
  });
});

describe('resolveBuiltinParserId', () => {
  it('maps legacy builtin ids to their current id', () => {
    expect(resolveBuiltinParserId('naive')).toBe('general');
  });

  it('passes through current and unknown ids unchanged', () => {
    expect(resolveBuiltinParserId('general')).toBe('general');
    expect(resolveBuiltinParserId('book')).toBe('book');
    expect(resolveBuiltinParserId('')).toBe('');
  });
});

describe('buildParserSelectValue', () => {
  it('resolves legacy builtin ids so old saved values match an option', () => {
    expect(
      buildParserSelectValue({
        parseType: ParseType.BuiltIn,
        parserId: 'naive',
      }),
    ).toBe('builtin:general');
  });

  it('builds the value from the ids of the active parse type', () => {
    expect(
      buildParserSelectValue({
        parseType: ParseType.BuiltIn,
        parserId: 'book',
        pipelineId: 'pipe-1',
        pipelineVersionId: 'v1',
      }),
    ).toBe('builtin:book');
    expect(
      buildParserSelectValue({
        parseType: ParseType.Pipeline,
        parserId: 'book',
        pipelineId: 'pipe-1',
        pipelineVersionId: 'v1',
      }),
    ).toBe('pipeline:pipe-1:v1');
  });

  it('returns undefined when the active id is empty', () => {
    expect(
      buildParserSelectValue({ parseType: ParseType.BuiltIn, parserId: '' }),
    ).toBeUndefined();
    expect(
      buildParserSelectValue({ parseType: ParseType.Pipeline }),
    ).toBeUndefined();
  });

  it('returns undefined for a pipeline without a concrete version', () => {
    // An unpinned binding has no leaf to select; the select shows its
    // placeholder until the latest-version prefill fills the field in.
    expect(
      buildParserSelectValue({
        parseType: ParseType.Pipeline,
        pipelineId: 'pipe-1',
      }),
    ).toBeUndefined();
    expect(
      buildParserSelectValue({
        parseType: ParseType.Pipeline,
        pipelineId: 'pipe-1',
        pipelineVersionId: '',
      }),
    ).toBeUndefined();
  });
});
