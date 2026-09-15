import { findEntityDisplayNameByKeyword } from './adapters';

describe('findEntityDisplayNameByKeyword', () => {
  const entities = [
    { id: 'e1', name: 'swallow' },
    { id: 'e2', name: 'Swan', aliases: ['swan lake', ' SWAN '] },
    { id: 'e3', name: 'owl tree' },
  ];

  it('matches an entity name case-insensitively and returns its canonical name', () => {
    expect(findEntityDisplayNameByKeyword(entities, 'swallow')).toBe('swallow');
    expect(findEntityDisplayNameByKeyword(entities, '  SWALLOW ')).toBe(
      'swallow',
    );
  });

  it('matches an alias and returns the canonical entity name', () => {
    expect(findEntityDisplayNameByKeyword(entities, 'swan lake')).toBe('Swan');
    expect(findEntityDisplayNameByKeyword(entities, 'swan')).toBe('Swan');
  });

  it('returns an empty string for partial matches, unknown or empty keywords', () => {
    expect(findEntityDisplayNameByKeyword(entities, 'swal')).toBe('');
    expect(findEntityDisplayNameByKeyword(entities, 'tree')).toBe('');
    expect(findEntityDisplayNameByKeyword(entities, 'unknown')).toBe('');
    expect(findEntityDisplayNameByKeyword(entities, '')).toBe('');
    expect(findEntityDisplayNameByKeyword([], 'swallow')).toBe('');
  });
});
