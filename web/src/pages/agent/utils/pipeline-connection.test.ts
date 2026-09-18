import { Operator } from '@/constants/agent';
import {
  isChunkerOperator,
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
