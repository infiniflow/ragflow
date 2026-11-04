import { Operator } from '@/constants/agent';
import {
  buildPipelineNextOperators,
  isChunkerOperator,
  isSingleInstanceOperator,
  isValidGoPipelineConnection,
} from './pipeline-connection';

describe('isChunkerOperator', () => {
  it('treats TokenChunker and TitleChunker as chunkers', () => {
    expect(isChunkerOperator(Operator.TokenChunker)).toBe(true);
    expect(isChunkerOperator(Operator.TitleChunker)).toBe(true);
  });

  it('excludes every other operator, including legacy GeneralChunker', () => {
    expect(isChunkerOperator(Operator.Parser)).toBe(false);
    expect(isChunkerOperator(Operator.GeneralChunker)).toBe(false);
    expect(isChunkerOperator(Operator.Tokenizer)).toBe(false);
    expect(isChunkerOperator(Operator.Extractor)).toBe(false);
    expect(isChunkerOperator(Operator.Compiler)).toBe(false);
    expect(isChunkerOperator(Operator.File)).toBe(false);
  });
});

describe('isValidGoPipelineConnection', () => {
  it('allows a Parser to feed a chunker', () => {
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.TokenChunker),
    ).toBe(true);
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.TitleChunker),
    ).toBe(true);
  });

  it('rejects a Parser feeding any non-chunker operator', () => {
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.Tokenizer),
    ).toBe(false);
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.Extractor),
    ).toBe(false);
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.Compiler),
    ).toBe(false);
    expect(isValidGoPipelineConnection(Operator.Parser, Operator.Parser)).toBe(
      false,
    );
    expect(
      isValidGoPipelineConnection(Operator.Parser, Operator.GeneralChunker),
    ).toBe(false);
  });

  it('rejects a chunker feeding another chunker', () => {
    expect(
      isValidGoPipelineConnection(Operator.TokenChunker, Operator.TitleChunker),
    ).toBe(false);
    expect(
      isValidGoPipelineConnection(Operator.TitleChunker, Operator.TokenChunker),
    ).toBe(false);
    expect(
      isValidGoPipelineConnection(Operator.TokenChunker, Operator.TokenChunker),
    ).toBe(false);
  });

  it('allows a chunker to feed non-chunker operators', () => {
    expect(
      isValidGoPipelineConnection(Operator.TokenChunker, Operator.Tokenizer),
    ).toBe(true);
    expect(
      isValidGoPipelineConnection(Operator.TitleChunker, Operator.Extractor),
    ).toBe(true);
    expect(
      isValidGoPipelineConnection(Operator.TokenChunker, Operator.Compiler),
    ).toBe(true);
  });

  it('leaves unrelated pairs unrestricted', () => {
    expect(isValidGoPipelineConnection(Operator.File, Operator.Parser)).toBe(
      true,
    );
    expect(
      isValidGoPipelineConnection(Operator.Tokenizer, Operator.Extractor),
    ).toBe(true);
    expect(
      isValidGoPipelineConnection(Operator.Extractor, Operator.Compiler),
    ).toBe(true);
  });
});

describe('isSingleInstanceOperator', () => {
  it('marks the one-instance pipeline operators', () => {
    expect(isSingleInstanceOperator(Operator.Parser)).toBe(true);
    expect(isSingleInstanceOperator(Operator.Tokenizer)).toBe(true);
    expect(isSingleInstanceOperator(Operator.Compiler)).toBe(true);
    expect(isSingleInstanceOperator(Operator.TokenChunker)).toBe(true);
    expect(isSingleInstanceOperator(Operator.TitleChunker)).toBe(true);
    expect(isSingleInstanceOperator(Operator.GeneralChunker)).toBe(true);
  });

  it('leaves multi-instance operators unchecked', () => {
    expect(isSingleInstanceOperator(Operator.Extractor)).toBe(false);
    expect(isSingleInstanceOperator(Operator.Agent)).toBe(false);
  });
});

describe('buildPipelineNextOperators', () => {
  const noOperator = () => false;

  it('always offers Extractor, which is multi-instance', () => {
    const hasExtractor = (operator: Operator) =>
      operator === Operator.Extractor;
    expect(
      buildPipelineNextOperators(undefined, true, hasExtractor).operators,
    ).toContain(Operator.Extractor);
  });

  it('offers Compiler only when none is on the canvas', () => {
    const offered = buildPipelineNextOperators(
      undefined,
      true,
      noOperator,
    ).operators;
    expect(offered).toContain(Operator.Compiler);

    const hasCompiler = (operator: Operator) => operator === Operator.Compiler;
    expect(
      buildPipelineNextOperators(undefined, true, hasCompiler).operators,
    ).not.toContain(Operator.Compiler);
  });

  it('hides the whole chunker group once any chunker is on the canvas', () => {
    const hasTokenChunker = (operator: Operator) =>
      operator === Operator.TokenChunker;
    const result = buildPipelineNextOperators(
      Operator.TokenChunker,
      false,
      hasTokenChunker,
    );
    expect(result.showChunker).toBe(false);
    expect(result.chunkerOperators).toEqual([]);
  });

  it('offers both chunker variants while the chunker slot is free', () => {
    const result = buildPipelineNextOperators(
      Operator.Tokenizer,
      true,
      noOperator,
    );
    expect(result.showChunker).toBe(true);
    expect(result.chunkerOperators).toEqual([
      Operator.TokenChunker,
      Operator.TitleChunker,
    ]);
  });
});
