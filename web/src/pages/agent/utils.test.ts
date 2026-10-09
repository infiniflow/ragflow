import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { Operator } from './constant';
import {
  generateNodeNamesWithIncreasingIndex,
  isEmptyMessageContent,
  receiveMessageError,
  transformTokenChunkerParams,
} from './utils';

describe('transformTokenChunkerParams', () => {
  it('keeps overlapped_percent and delimiters when delimiter_mode is one', () => {
    // Regression: saving with delimiter_mode='one' used to zero these fields,
    // so a reload showed overlapped_percent=0 and delimiters=["\n"].
    const result = transformTokenChunkerParams({
      delimiter_mode: 'one',
      chunk_token_size: 512,
      overlapped_percent: 9,
      image_table_context_window: 81,
      delimiters: [{ value: '\n' }, { value: '!' }, { value: '。' }],
      children_delimiters: [],
      enable_children: false,
    } as any);

    expect(result.overlapped_percent).toBeCloseTo(0.09);
    expect(result.delimiters).toEqual(['\n', '!', '。']);
    expect(result.delimiter_mode).toBe('one');
  });

  it('converts form values to api format in delimiter mode', () => {
    const result = transformTokenChunkerParams({
      delimiter_mode: 'delimiter',
      chunk_token_size: 512,
      overlapped_percent: 30,
      image_table_context_window: 81,
      delimiters: [{ value: '\n' }, { value: '' }],
      children_delimiters: [{ value: '|' }],
      enable_children: true,
    } as any);

    expect(result.overlapped_percent).toBeCloseTo(0.3);
    expect(result.delimiters).toEqual(['\n']);
    expect(result.children_delimiters).toEqual(['|']);
    expect(result.table_context_size).toBe(81);
    expect(result.image_context_size).toBe(81);
  });
});

describe('receiveMessageError', () => {
  it('accepts successful SSE events without an application code', () => {
    expect(
      receiveMessageError({
        response: { status: 200 },
        data: { event: 'workflow_finished' },
      }),
    ).toBe(false);
  });
  it('rejects application errors returned with HTTP 200', () => {
    expect(
      receiveMessageError({ response: { status: 200 }, data: { code: 102 } }),
    ).toBe(true);
    expect(
      receiveMessageError({ response: { status: 200 }, data: { code: 0 } }),
    ).toBe(false);
  });
});

describe('Message component content validation', () => {
  describe('isEmptyMessageContent', () => {
    it('treats missing, non-array and blank-only content as empty', () => {
      expect(isEmptyMessageContent()).toBe(true);
      expect(isEmptyMessageContent(null)).toBe(true);
      expect(isEmptyMessageContent('hello')).toBe(true);
      expect(isEmptyMessageContent([])).toBe(true);
      expect(isEmptyMessageContent(['', ' \t '])).toBe(true);
      // Non-string entries never satisfy the backend either.
      expect(isEmptyMessageContent([123])).toBe(true);
    });

    it('accepts content with at least one non-blank string entry', () => {
      expect(isEmptyMessageContent(['hi'])).toBe(false);
      expect(isEmptyMessageContent(['', '{begin@query}'])).toBe(true);
      expect(isEmptyMessageContent(['  text  '])).toBe(false);
    });
  });
});

describe('generateNodeNamesWithIncreasingIndex', () => {
  const createNamedNode = (name: string) =>
    ({
      id: `${Operator.Retrieval}:${name}`,
      type: 'ragNode',
      position: { x: 0, y: 0 },
      data: { label: Operator.Retrieval, name, form: {} },
    }) as RAGFlowNodeType;

  it('uses the bare name for the first operator of a type', () => {
    expect(generateNodeNamesWithIncreasingIndex('Retrieval', [])).toBe(
      'Retrieval',
    );
  });

  it('appends an index only from the second operator on', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
      ]),
    ).toBe('Retrieval_1');
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
        createNamedNode('Retrieval_1'),
      ]),
    ).toBe('Retrieval_2');
  });

  it('fills the gap between existing indexes', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval'),
        createNamedNode('Retrieval_2'),
      ]),
    ).toBe('Retrieval_1');
  });

  it('does not backfill index 0 when only suffixed names exist', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval_1'),
      ]),
    ).toBe('Retrieval_2');
  });

  it('treats a legacy _0 name as the first operator', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Retrieval_0'),
      ]),
    ).toBe('Retrieval_1');
  });

  it('ignores nodes of other types and non-indexed names', () => {
    expect(
      generateNodeNamesWithIncreasingIndex('Retrieval', [
        createNamedNode('Message'),
        createNamedNode('Retrieval_beta'),
        createNamedNode('Retrieval_1_extra'),
      ]),
    ).toBe('Retrieval');
  });
});
