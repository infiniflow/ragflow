import { findCitationChunk, parseCitationIndex } from '@/utils/citation-utils';

describe('parseCitationIndex', () => {
  it('keeps bare numeric markers as numbers', () => {
    expect(parseCitationIndex('[12]')).toBe(12);
  });

  it('keeps explicit ID markers as strings', () => {
    expect(parseCitationIndex('[ID:12]')).toBe('12');
    expect(parseCitationIndex('[ID:9ac9b07178c3951a]')).toBe(
      '9ac9b07178c3951a',
    );
  });

  it('preserves all-digit hex ids instead of converting them', () => {
    expect(parseCitationIndex('[ID:0000123456789012]')).toBe(
      '0000123456789012',
    );
    expect(parseCitationIndex('[ID:1234567890123456]')).toBe(
      '1234567890123456',
    );
  });
});

describe('findCitationChunk', () => {
  const chunks = [
    { id: 'aaa', document_id: 'doc-a' },
    { id: 'bbb', document_id: 'doc-b' },
    { id: 'ccc', document_id: 'doc-c' },
  ];

  it('finds the chunk by id even when citation order differs', () => {
    expect(findCitationChunk(chunks, 'ccc', 0)).toEqual(chunks[2]);
    expect(findCitationChunk(chunks, 'bbb', 2)).toEqual(chunks[1]);
  });

  it('prefers the index key for record-shaped references', () => {
    const record = { '0': chunks[0], '1': chunks[1] };
    expect(findCitationChunk(record, 1, 0)).toEqual(chunks[1]);
  });

  it('falls back to the display position when the id is absent', () => {
    expect(findCitationChunk(chunks, 'missing', 1)).toEqual(chunks[1]);
  });

  it('returns undefined when nothing matches', () => {
    expect(findCitationChunk(undefined, 'aaa', 0)).toBeUndefined();
    expect(findCitationChunk([], 'aaa', 3)).toBeUndefined();
  });
});
