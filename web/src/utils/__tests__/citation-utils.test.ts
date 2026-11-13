import type { IReferenceChunk } from '@/interfaces/database/chat';
import reactStringReplace from 'react-string-replace';
import {
  buildCitationDisplayMap,
  citationWholeMarkerReg,
  parseCitationIndex,
  resolveCitationChunk,
} from '../citation-utils';

const buildChunk = (id: string): IReferenceChunk => ({
  id,
  content: null,
  document_id: `doc-${id}`,
  document_name: `${id}.txt`,
  dataset_id: 'kb-1',
  image_id: '',
  similarity: 1,
  vector_similarity: 1,
  term_similarity: 1,
  positions: [],
});

const HappyPrince = buildChunk('a2d067519c9e0db1');
const Nightingale = buildChunk('b3e178620da1fc42');

describe('parseCitationIndex', () => {
  it('parses numeric markers as numbers', () => {
    expect(parseCitationIndex('[ID:2]')).toBe(2);
    expect(parseCitationIndex('[3]')).toBe(3);
  });

  it('keeps short explicit numeric ids as positional indexes', () => {
    expect(parseCitationIndex('[ID:0]')).toBe(0);
    expect(parseCitationIndex('[ID:12]')).toBe(12);
    expect(parseCitationIndex('[ID:123456789012345]')).toBe(123456789012345);
  });

  it('parses hex chunk-id markers as strings', () => {
    expect(parseCitationIndex('[ID:a2d067519c9e0db1]')).toBe(
      'a2d067519c9e0db1',
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

  it('keeps short leading-zero ids as strings', () => {
    expect(parseCitationIndex('[ID:0123]')).toBe('0123');
  });
});

describe('resolveCitationChunk', () => {
  // The payload ships in retrieval order: Happy Prince ranked first.
  const chunks = [HappyPrince, Nightingale];

  it('resolves a numeric key positionally', () => {
    expect(resolveCitationChunk(chunks, 0)).toBe(HappyPrince);
    expect(resolveCitationChunk(chunks, 1)).toBe(Nightingale);
  });

  it('returns undefined for an out-of-range numeric key', () => {
    expect(resolveCitationChunk(chunks, 5)).toBeUndefined();
  });

  it('resolves a chunk-id key by id, not by citation order', () => {
    // The answer cites Nightingale first; the chunk must still be the one
    // the marker names, not chunks[0] (the mis-citation regression).
    expect(resolveCitationChunk(chunks, 'b3e178620da1fc42')).toBe(Nightingale);
    expect(resolveCitationChunk(chunks, 'a2d067519c9e0db1')).toBe(HappyPrince);
  });

  it('falls back to chunk_id when the id field is absent', () => {
    const legacy = [
      { chunk_id: 'b3e178620da1fc42' } as unknown as IReferenceChunk,
    ];
    expect(resolveCitationChunk(legacy, 'b3e178620da1fc42')).toBe(legacy[0]);
  });

  it('returns undefined for an unknown chunk id instead of guessing positionally', () => {
    expect(resolveCitationChunk(chunks, 'ffffffffffffffff')).toBeUndefined();
  });

  it('resolves ids inside a Record-shaped payload', () => {
    const record = {
      a2d067519c9e0db1: HappyPrince,
      b3e178620da1fc42: Nightingale,
    };
    expect(resolveCitationChunk(record, 'b3e178620da1fc42')).toBe(Nightingale);
  });

  it('tolerates an empty payload', () => {
    expect(resolveCitationChunk(undefined, 0)).toBeUndefined();
    expect(resolveCitationChunk(null, 'a2d067519c9e0db1')).toBeUndefined();
  });
});

describe('citationWholeMarkerReg', () => {
  it('hands react-string-replace the whole marker, not the bare id', () => {
    const seen: string[] = [];
    reactStringReplace(
      'see [ID:0000123456789012] here',
      citationWholeMarkerReg,
      (match) => {
        seen.push(match);
        return match;
      },
    );

    expect(seen).toEqual(['[ID:0000123456789012]']);
    expect(parseCitationIndex(seen[0])).toBe('0000123456789012');
  });
});

describe('buildCitationDisplayMap', () => {
  it('numbers unique markers by first occurrence, whatever their key', () => {
    const map = buildCitationDisplayMap(
      'nightingale [ID:b3e178620da1fc42] then happy prince [ID:a2d067519c9e0db1] again [ID:b3e178620da1fc42]',
    );
    expect(map.get('b3e178620da1fc42')).toBe(0);
    expect(map.get('a2d067519c9e0db1')).toBe(1);
    expect(map.size).toBe(2);
  });

  it('uses numeric keys for short explicit numeric ids', () => {
    const map = buildCitationDisplayMap('[ID:0] [ID:1] [ID:0]');
    expect([...map.entries()]).toEqual([
      [0, 0],
      [1, 1],
    ]);
  });
});
