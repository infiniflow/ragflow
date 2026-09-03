import { Operator } from './constant';
import {
  getEmptyMessageNodeNames,
  isEmptyMessageContent,
  transformTokenChunkerParams,
} from './utils';

const createMessageNode = (name: string, content: unknown) => ({
  id: `${Operator.Message}:${name}`,
  type: 'ragNode',
  position: { x: 0, y: 0 },
  data: { label: Operator.Message, name, form: { content } },
});

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

  describe('getEmptyMessageNodeNames', () => {
    it('flags only Message nodes whose content is empty', () => {
      const nodes = [
        createMessageNode('回复消息_0', ['']),
        createMessageNode('回复消息_1', ['ok']),
        {
          id: `${Operator.Agent}:x`,
          type: 'ragNode',
          position: { x: 0, y: 0 },
          data: { label: Operator.Agent, name: '智能体_0', form: {} },
        },
      ];

      expect(getEmptyMessageNodeNames(nodes as any)).toEqual(['回复消息_0']);
    });
  });
});
