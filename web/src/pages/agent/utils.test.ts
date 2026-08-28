import { transformTokenChunkerParams } from './utils';

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
