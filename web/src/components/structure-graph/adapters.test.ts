import { CompilationTemplateKind } from '@/constants/compilation';
import {
  adaptMindMapToIndentedTree,
  findEntityDisplayNameByKeyword,
} from './adapters';

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

describe('adaptMindMapToIndentedTree', () => {
  it('ignores self-referencing relations instead of hanging the tree build', () => {
    // Regression: a `root -> root` relation recorded root as its own parent,
    // and the cycle-avoidance walk then spun on it forever — the mind map
    // stayed on the loading skeleton. The self-loop must be skipped and the
    // remaining relations still attach normally.
    const tree = adaptMindMapToIndentedTree({
      kind: CompilationTemplateKind.MindMap,
      template_id: 't1',
      template_name: 'mindmap',
      entities: [{ name: 'root' }, { name: 'child a' }, { name: 'child b' }],
      relations: [
        { from: 'root', to: 'root', type: 'related' },
        { from: 'root', to: 'child a', type: 'related' },
        { from: 'root', to: 'child b', type: 'related' },
      ],
    });

    expect(tree.id).toBe('root');
    expect(tree.children?.map((child) => child.id)).toEqual([
      'child a',
      'child b',
    ]);
  });
});
