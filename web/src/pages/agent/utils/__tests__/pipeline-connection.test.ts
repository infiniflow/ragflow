import { Operator } from '@/constants/agent';
import {
  buildPipelineNextOperators,
  hasPipelineNextOperators,
  isSingleInstanceOperator,
} from '../pipeline-connection';

const noOperators = () => false;
const onlyExtractor = (operator: Operator) => operator === Operator.Extractor;

describe('buildPipelineNextOperators', () => {
  it('offers only the next stages allowed by the source operator', () => {
    const { operators } = buildPipelineNextOperators(
      Operator.TokenChunker,
      noOperators,
    );
    expect(operators).toContain(Operator.Extractor);
    expect(operators).toContain(Operator.Tokenizer);
    expect(operators).toContain(Operator.Compiler);
    expect(operators).not.toContain(Operator.Parser);
  });

  it('hides Extractor when one is already on the canvas', () => {
    const { operators } = buildPipelineNextOperators(
      Operator.TokenChunker,
      onlyExtractor,
    );
    expect(operators).not.toContain(Operator.Extractor);
    expect(operators).toEqual(
      expect.arrayContaining([Operator.Tokenizer, Operator.Compiler]),
    );
    expect(operators).not.toContain(Operator.Parser);
  });

  it('still hides the chunker group from an Extractor source', () => {
    const { operators } = buildPipelineNextOperators(
      Operator.Extractor,
      onlyExtractor,
    );
    expect(operators).not.toContain(Operator.Extractor);
    expect(operators).not.toContain(Operator.TokenChunker);
    expect(operators).not.toContain(Operator.TitleChunker);
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
