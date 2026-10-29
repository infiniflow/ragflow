import { act, render } from '@testing-library/react';
import { ParserTreeSelect } from './index';

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

const VersionList = [
  { id: 'v1', title: 'v1 title', created_at: '', alias: 'jin' },
];

// Index-aligned with the sorted canvas ids the hook requests: pipe-1 is the
// selected pipeline and always fetched; pipe-2 appears once its branch is
// expanded in a test.
let mockVersionQueries: any[] = [];
jest.mock('@/hooks/use-agent-request', () => ({
  useFetchBuiltinPipelines: () => ({
    options: [{ label: 'General', value: 'general' }],
    loading: false,
  }),
  useFetchAgentList: () => ({
    data: {
      canvas: [
        { id: 'pipe-1', title: 'My Flow', type: 'pipeline' },
        { id: 'pipe-2', title: 'Empty Flow', type: 'pipeline' },
        // AgentListItemType.CompilationTemplateGroup — referenced as a literal
        // because the jest.mock factory cannot capture an imported enum.
        { id: 'grp-1', title: 'Group', type: 'compilation_template_group' },
      ],
      total: 2,
    },
    loading: false,
  }),
  useFetchCanvasVersions: () => mockVersionQueries,
}));

beforeEach(() => {
  for (const key of Object.keys(mockTreeProps)) delete mockTreeProps[key];
  mockVersionQueries = [{ isSuccess: true, data: VersionList }];
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

  it('excludes compilation template groups from the pipeline branches', () => {
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

  it('caps the alias at 30% only in the trigger display, not in the row', () => {
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
    expect(findAliasSpan(row)?.className).not.toContain('max-w-');
  });

  it('auto-expands the branch holding the selection', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    expect([...mockTreeProps.expandedIds]).toEqual(['pipeline:pipe-1']);
  });

  it('keeps an unrequested branch non-selectable via a loading placeholder', () => {
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    const emptyFlow = mockTreeProps.data[1];

    // pipe-2's versions were never requested, so it must not collapse into a
    // selectable leaf — a disabled placeholder child keeps it a branch.
    expect(emptyFlow.children).toHaveLength(1);
    expect(emptyFlow.children[0].id).toBe('pipeline:pipe-2:loading');
    expect(emptyFlow.children[0].disabled).toBe(true);
  });

  it('lazily fetches versions for a freshly expanded branch', () => {
    mockVersionQueries = [
      { isSuccess: true, data: VersionList },
      { isSuccess: false, data: undefined },
    ];
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    act(() => {
      mockTreeProps.onExpandedChange(
        new Set(['pipeline:pipe-1', 'pipeline:pipe-2']),
      );
    });

    const emptyFlow = mockTreeProps.data[1];
    expect(emptyFlow.children[0].id).toBe('pipeline:pipe-2:loading');
    expect([...mockTreeProps.loadingIds]).toContain('pipeline:pipe-2');
  });

  it('shows a disabled empty row for a branch with no published versions', () => {
    mockVersionQueries = [
      { isSuccess: true, data: VersionList },
      { isSuccess: true, data: [] },
    ];
    render(
      <ParserTreeSelect value="pipeline:pipe-1:v1" onChange={jest.fn()} />,
    );
    act(() => {
      mockTreeProps.onExpandedChange(
        new Set(['pipeline:pipe-1', 'pipeline:pipe-2']),
      );
    });

    const emptyFlow = mockTreeProps.data[1];
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
    mockVersionQueries = [{ isSuccess: false, data: undefined }];
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
