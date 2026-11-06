import { Operator } from '@/constants/agent';
import {
  buildPipelineNextOperators,
  hasPipelineNextOperators,
  isSingleInstanceOperator,
} from '../pipeline-connection';

const noOperators = () => false;
const onlyExtractor = (operator: Operator) => operator === Operator.Extractor;

describe('buildPipelineNextOperators', () => {
  it('offers Extractor when the canvas has none', () => {
    const { operators } = buildPipelineNextOperators(
      Operator.TokenChunker,
      noOperators,
    );
    expect(operators).toContain(Operator.Extractor);
  });

  it('hides Extractor when one is already on the canvas', () => {
    const { operators } = buildPipelineNextOperators(
      Operator.TokenChunker,
      onlyExtractor,
    );
    expect(operators).not.toContain(Operator.Extractor);
    // Other operators are unaffected.
    expect(operators).toEqual(
      expect.arrayContaining([
        Operator.Parser,
        Operator.Tokenizer,
        Operator.Compiler,
      ]),
    );
  });

  it('still hides the chunker group from an Extractor source', () => {
    const { operators, showChunker } = buildPipelineNextOperators(
      Operator.Extractor,
      onlyExtractor,
    );
    expect(operators).not.toContain(Operator.Extractor);
    expect(showChunker).toBe(false);
  });
});

describe('hasPipelineNextOperators', () => {
  it('reports an empty menu when only single-instance operators remain', () => {
    const everythingTaken = () => true;
    expect(hasPipelineNextOperators(Operator.Extractor, everythingTaken)).toBe(
      false,
    );
  });
});

describe('isSingleInstanceOperator', () => {
  it('treats Extractor as single-instance so duplication is disabled', () => {
    expect(isSingleInstanceOperator(Operator.Extractor)).toBe(true);
  });
});
