import { buildNavTreeData } from './nav-tree';

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
