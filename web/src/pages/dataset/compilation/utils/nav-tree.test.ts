import { adaptTreeToTreeData } from '@/components/structure-graph/adapters';
import { CompilationTemplateKind } from '@/constants/compilation';

import { buildNavTreeData, nestNavSearchHits } from './nav-tree';

jest.mock('@/components/structure-graph/adapters', () => ({
  adaptPageIndexToTreeData: jest.fn(() => []),
  adaptTreeToTreeData: jest.fn(() => []),
  getEntityDisplayName: jest.fn((entity) => entity.name ?? ''),
}));

function cluster(name: string) {
  return {
    name,
    description: '',
    doc_count: 1,
    type: 'cluster',
    has_children: true,
  };
}

const TreeOptions = {
  structureMap: {},
  onNodeClick: jest.fn(),
  onNodeExpand: jest.fn(),
  loadingPlaceholder: 'Loading...',
  errorPlaceholder: 'Failed to load child nodes',
};

describe('buildNavTreeData', () => {
  it('renders an error placeholder when a child load failed', () => {
    const data = buildNavTreeData([cluster('cluster-a')], {
      ...TreeOptions,
      childrenMap: {},
      childrenErrorParents: { 'cluster-a': true },
    });

    expect(data[0].children).toEqual([
      { id: 'cluster-a/__error__', name: 'Failed to load child nodes' },
    ]);
  });

  it('does not keep the loading placeholder after an empty child list is cached', () => {
    const data = buildNavTreeData([cluster('cluster-a')], {
      ...TreeOptions,
      childrenMap: { 'cluster-a': [] },
    });

    expect(data[0].children).toBeUndefined();
  });
});

function doc(docId: string, name: string, parentKwd: string, matched = true) {
  return {
    name,
    description: '',
    doc_count: 1,
    type: 'doc',
    doc_id: docId,
    has_children: false,
    parent_kwd: parentKwd,
    matched,
  };
}

function clusterIn(name: string, parentKwd: string, docCount = 1) {
  return {
    name,
    description: '',
    doc_count: docCount,
    type: 'cluster',
    has_children: true,
    parent_kwd: parentKwd,
    matched: false,
  };
}

describe('nestNavSearchHits', () => {
  it('turns the flat hits plus their cluster path into a forest', () => {
    const { roots, children } = nestNavSearchHits([
      clusterIn('三国 11111111', 'root'),
      clusterIn('名将 22222222', '三国 11111111'),
      doc('d1', '三国人物.pdf', '名将 22222222'),
      doc('d2', '其他.pdf', '三国 11111111'),
    ]);

    expect(roots.map((node) => node.name)).toEqual(['三国 11111111']);
    expect(children['cluster:三国 11111111'].map((node) => node.name)).toEqual([
      '名将 22222222',
      '其他.pdf',
    ]);
    expect(children['cluster:名将 22222222'].map((node) => node.name)).toEqual([
      '三国人物.pdf',
    ]);
  });

  it('keeps a hit whose parent is not in the result set as its own root', () => {
    const { roots, children } = nestNavSearchHits([
      doc('d9', '孤儿.pdf', 'missing-cluster'),
    ]);

    expect(roots.map((node) => node.name)).toEqual(['孤儿.pdf']);
    expect(children).toEqual({});
  });
});

describe('buildNavTreeData (search mode)', () => {
  const SearchOptions = { ...TreeOptions, childrenMap: {}, searchMode: true };

  it('renders ONE tree per root cluster holding only the matched branches', () => {
    const data = buildNavTreeData(
      [
        clusterIn('三国 11111111', 'root', 2),
        clusterIn('名将 22222222', '三国 11111111'),
        doc('d1', '三国人物.pdf', '名将 22222222'),
        doc('d2', '其他.pdf', '三国 11111111'),
      ],
      SearchOptions,
    );

    expect(data).toHaveLength(1);
    expect(data[0].id).toBe('cluster:三国 11111111');
    expect(data[0].hasChildren).toBe(true);
    // Child ids are prefixed with the parent's id so they stay unique across
    // the forest.
    expect(data[0].children?.map((child) => child.id)).toEqual([
      'cluster:三国 11111111/cluster:名将 22222222',
      'cluster:三国 11111111/doc:d2',
    ]);
    // The deeper match sits under its matched ancestor, not beside it.
    expect(data[0].children?.[0].children?.map((child) => child.id)).toEqual([
      'cluster:三国 11111111/cluster:名将 22222222/doc:d1',
    ]);
  });

  it('does not mount a pending structure placeholder under a matched leaf', () => {
    // The branches mount already expanded, so a placeholder would never be
    // replaced (mount-time expansion fires no onExpand, which fetches the graph).
    const data = buildNavTreeData([doc('d1', '三国人物.pdf', 'root')], {
      ...SearchOptions,
      structureMap: {},
    });

    expect(data).toHaveLength(1);
    expect(data[0].children).toBeUndefined();
    expect(data[0].onExpand).toEqual(expect.any(Function));
  });

  it('mounts the fetched structure graph under a matched leaf', () => {
    jest
      .mocked(adaptTreeToTreeData)
      .mockReturnValueOnce([
        { id: 'e1', name: '刘备', children: [{ id: 'e2', name: '关羽' }] },
      ]);
    const onEntityClick = jest.fn();
    const data = buildNavTreeData([doc('d1', '三国人物.pdf', 'root')], {
      ...SearchOptions,
      onEntityClick,
      structureMap: {
        d1: [
          {
            template_id: 't1',
            template_name: '结构树',
            kind: CompilationTemplateKind.Tree,
            entities: [
              { id: 'e1', name: '刘备', description: '蜀汉开国皇帝' },
              { id: 'e2', name: '关羽' },
            ],
            relations: [],
          },
        ],
      },
    });

    expect(data).toHaveLength(1);
    expect(data[0].hasChildren).toBe(true);
    // Adapter ids are re-prefixed as <document id>/<template id>/<entity id>, so
    // they stay unique across the forest, and the hit stays expandable.
    expect(data[0].children?.map((child) => child.id)).toEqual([
      'doc:d1/t1/e1',
    ]);
    expect(data[0].children?.[0].children?.map((child) => child.id)).toEqual([
      'doc:d1/t1/e2',
    ]);
    expect(data[0].onExpand).toEqual(expect.any(Function));
    // Clicking an entity reports its document, display name and description —
    // the same payload the non-search tree sends.
    data[0].children?.[0].onClick?.();
    expect(onEntityClick).toHaveBeenCalledWith(
      expect.objectContaining({ doc_id: 'd1' }),
      '刘备',
      '蜀汉开国皇帝',
    );
  });

  it('leaves the matched leaf a leaf when the fetched graph holds no entity', () => {
    // The adapter is mocked to return no items, so a fetched-but-empty graph
    // renders no children instead of an empty expandable branch.
    const data = buildNavTreeData([doc('d1', '三国人物.pdf', 'root')], {
      ...SearchOptions,
      structureMap: {
        d1: [
          {
            template_id: 't1',
            template_name: '结构树',
            kind: CompilationTemplateKind.Tree,
            entities: [],
            relations: [],
          },
        ],
      },
    });

    expect(data[0].children).toBeUndefined();
    expect(data[0].onExpand).toEqual(expect.any(Function));
  });
});
