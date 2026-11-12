import { act, render } from '@testing-library/react';
import { ParserTreeSelect } from './index';
import { filterTreeNodes } from '../tree-select-utils';

const mockTreeProps: any = {};
jest.mock('@/components/tree-select', () => ({
  TreeSelect: (props: any) => {
    Object.assign(mockTreeProps, props);
    return null;
  },
}));

jest.mock('@/components/ui/tooltip', () => ({
  RAGFlowTooltip: ({ children }: any) => children,
}));

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

const VersionList = [{ id: 'v1', name: 'v1 title', alias: 'jin' }];
let mockCatalog: any[] = [];
let mockCatalogLoading = false;
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: () => ({
    options: [{ label: 'General', value: 'general' }],
    loading: false,
  }),
}));
jest.mock('@/hooks/use-pipeline-version-request', () => ({
  useFetchPipelineVersionCatalog: () => ({
    data: mockCatalog,
    loading: mockCatalogLoading,
  }),
}));

beforeEach(() => {
  for (const key of Object.keys(mockTreeProps)) delete mockTreeProps[key];
  mockCatalog = [
    { id: 'pipe-1', name: 'My Flow', versions: VersionList },
    { id: 'pipe-2', name: 'Empty Flow', versions: [] },
  ];
  mockCatalogLoading = false;
});

describe('ParserTreeSelect', () => {
  it('builds pipeline branches with version leaves and builtin root leaves', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    const [branch, , builtin] = mockTreeProps.data;

    expect(branch.id).toBe('pipeline:pipe-1');
    expect(branch.title).toBe('My Flow');
    expect(branch.children).toHaveLength(1);
    expect(branch.children[0].id).toBe('pipeline:pipe-1:v1');

    expect(builtin.id).toBe('builtin:general');
    expect(builtin.children).toBeUndefined();
  });

  it('builds branches directly from the dataflow catalog', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    const ids = mockTreeProps.data.map((node: any) => node.id);
    expect(ids).toEqual([
      'pipeline:pipe-1',
      'pipeline:pipe-2',
      'builtin:general',
    ]);
  });

  it('renders the alias as secondary text and keeps it searchable', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    const leaf = mockTreeProps.data[0].children[0];

    expect(leaf.title).toBe('v1 title jin');
    expect(leaf.searchKeywords).toEqual(['jin']);

    const { container } = render(<>{leaf.label}</>);
    expect(container.textContent).toContain('v1 title');
    expect(container.textContent).toContain('jin');
  });

  it('caps the alias in both the trigger display and the row', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    const leaf = mockTreeProps.data[0].children[0];
    expect(leaf.data).toEqual({ title: 'v1 title', alias: 'jin' });

    // textContent is matched exactly so the title span ('v1 title') and the
    // wrapper ('v1 titlejin') don't collide with the alias span ('jin').
    const findAliasSpan = (container: HTMLElement) =>
      Array.from(container.querySelectorAll('span')).find(
        (el) => el.textContent === 'jin',
      );

    const { container: trigger } = render(
      <>{mockTreeProps.renderSelected(leaf)}</>,
    );
    expect(findAliasSpan(trigger)?.className).toContain('max-w-[30%]');

    const { container: row } = render(<>{leaf.label}</>);
    expect(findAliasSpan(row)?.className).toContain('max-w-60');
  });

  it('auto-expands the branch holding the selection', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    expect([...mockTreeProps.expandedIds]).toEqual(['pipeline:pipe-1']);
  });

  it('searches a version alias in an unexpanded pipeline locally', () => {
    render(<ParserTreeSelect value={undefined} onChange={jest.fn()} />);
    expect([...mockTreeProps.expandedIds]).toEqual([]);
    const matches = filterTreeNodes(mockTreeProps.data, 'JIN');
    expect(matches.map((node) => node.id)).toEqual(['pipeline:pipe-1']);
    expect(matches[0].children?.[0].id).toBe('pipeline:pipe-1:v1');
    act(() => mockTreeProps.onSearchChange('JIN'));
    expect([...mockTreeProps.expandedIds]).toEqual(['pipeline:pipe-1']);
  });

  it('searches pipeline names and version names locally', () => {
    render(<ParserTreeSelect value={undefined} onChange={jest.fn()} />);
    for (const term of ['MY FLOW', 'V1 TITLE']) {
      const matches = filterTreeNodes(mockTreeProps.data, term);
      expect(matches.map((node) => node.id)).toEqual(['pipeline:pipe-1']);
      expect(matches[0].children?.[0].id).toBe('pipeline:pipe-1:v1');
    }
    expect(filterTreeNodes(mockTreeProps.data, 'unknown')).toEqual([]);
  });

  it('keeps version metadata when a branch is collapsed', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    act(() => mockTreeProps.onExpandedChange(new Set()));
    expect([...mockTreeProps.expandedIds]).toEqual([]);
    expect(mockTreeProps.data[0].children[0].id).toBe('pipeline:pipe-1:v1');
  });

  it('keeps a pipeline with no versions non-selectable', () => {
    render(<ParserTreeSelect value={undefined} onChange={jest.fn()} />);
    const emptyFlow = mockTreeProps.data[1];
    expect(emptyFlow.children).toHaveLength(1);
    expect(emptyFlow.children[0].id).toBe('pipeline:pipe-2:empty');
    expect(emptyFlow.children[0].disabled).toBe(true);
  });

  it('decodes a version leaf into kind + pipeline id + version id', () => {
    const onChange = jest.fn();
    render(<ParserTreeSelect value={undefined} onChange={onChange} />);
    mockTreeProps.onChange('pipeline:pipe-1:v1');
    expect(onChange).toHaveBeenCalledWith('pipeline', 'pipe-1', 'v1');
  });

  it('decodes a builtin leaf into kind + raw id', () => {
    const onChange = jest.fn();
    render(<ParserTreeSelect value={undefined} onChange={onChange} />);
    mockTreeProps.onChange('builtin:general');
    expect(onChange).toHaveBeenCalledWith('builtin', 'general', undefined);
  });

  it('emits null for an unrecognized selection', () => {
    const onChange = jest.fn();
    render(<ParserTreeSelect value={undefined} onChange={onChange} />);
    mockTreeProps.onChange('garbage');
    expect(onChange).toHaveBeenCalledWith(null, '');
  });

  it('marks the select as loading while the selected versions arrive', () => {
    mockCatalogLoading = true;
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    expect(mockTreeProps.loading).toBe(true);
  });

  it('shows the outdated icon only when the selected leaf exists', () => {
    const { rerender } = render(
      <ParserTreeSelect
        value="pipeline:pipe-1:v1"
        onChange={jest.fn()}
        outdated
      />,
    );
    expect(mockTreeProps.trailingAddon).toBeTruthy();

    // A pinned version missing from the loaded list renders the missing
    // display instead — the outdated icon would be noise next to it.
    rerender(
      <ParserTreeSelect
        value="pipeline:pipe-1:pruned"
        onChange={jest.fn()}
        outdated
      />,
    );
    expect(mockTreeProps.trailingAddon).toBeUndefined();
  });

  it('renders the outdated icon as a non-submit button inside forms', () => {
    render(
      <ParserTreeSelect
        value="pipeline:pipe-1:v1"
        onChange={jest.fn()}
        outdated
      />,
    );
    // The tooltip trigger renders a raw <button>; without an explicit
    // type="button" it defaults to submit and clicking the icon would submit
    // the surrounding form (dataset settings, document pipeline dialog).
    expect(mockTreeProps.trailingAddon.props.type).toBe('button');
  });

  it('flags the trigger border only while the owning field is invalid', () => {
    const { rerender } = render(
      <ParserTreeSelect
        value="pipeline:pipe-1:v1"
        onChange={jest.fn()}
        invalid
      />,
    );
    expect(mockTreeProps.className).toContain('border-destructive');

    rerender(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    expect(mockTreeProps.className ?? '').not.toContain('border-destructive');
  });
});
