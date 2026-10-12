import { render } from '@testing-library/react';
import { ParserSelect } from '@/components/parser-select';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

const mockSelectProps: any = { onChange: jest.fn() };
jest.mock('@/components/originui/select-with-search', () => ({
  SelectWithSearch: (props: any) => {
    mockSelectProps.value = props.value;
    mockSelectProps.options = props.options;
    mockSelectProps.loading = props.loading;
    mockSelectProps.placeholder = props.placeholder;
    mockSelectProps.onChange = props.onChange;
    return <select data-testid="mock-select" />;
  },
}));

jest.mock('@/hooks/use-parser-options', () => {
  const parserOptions = {
    options: [
      { value: 'pipeline:pipe-1', label: 'My Flow', kind: 'pipeline' },
      {
        value: 'builtin:general',
        label: 'General',
        kind: 'builtin',
      },
    ],
    loading: false,
  };
  return {
    __setLoading: (v: boolean) => {
      parserOptions.loading = v;
    },
    ParserOptionKind: { BuiltIn: 'builtin', Pipeline: 'pipeline' },
    useParserOptions: () => parserOptions,
    buildParserOptionValue: (kind: string, id: string) => `${kind}:${id}`,
    parseParserOptionValue: (v?: string) => {
      if (
        !v ||
        (v.startsWith('builtin:') === false &&
          v.startsWith('pipeline:') === false)
      ) {
        return null;
      }
      const idx = v.indexOf(':');
      return { kind: v.slice(0, idx), rawId: v.slice(idx + 1) };
    },
  };
});

// The mocked SelectWithSearch stores its (internal) onChange handler in
// mockSelectProps.onChange. Firing it exercises ParserSelect's decode logic,
// which calls the real user onChange we pass to ParserSelect below.
beforeEach(() => {
  mockSelectProps.onChange.mockClear?.();
});

describe('ParserSelect', () => {
  it('passes the prefixed value through to the underlying select', () => {
    render(<ParserSelect value="builtin:general" onChange={jest.fn()} />);
    expect(mockSelectProps.value).toBe('builtin:general');
  });

  it('passes the merged option list through', () => {
    render(<ParserSelect value={undefined} onChange={jest.fn()} />);
    expect(mockSelectProps.options).toHaveLength(2);
    expect(mockSelectProps.options[0].value).toBe('pipeline:pipe-1');
  });

  it('renders the builtin marker as a tag with searchable keywords', () => {
    render(<ParserSelect value={undefined} onChange={jest.fn()} />);
    const pipelineOption = mockSelectProps.options[0];
    const builtinOption = mockSelectProps.options[1];

    // Pipeline options keep their plain string label.
    expect(pipelineOption.label).toBe('My Flow');

    // Builtin options get a ReactNode label (label + tag) and explicit
    // keywords so cmdk search still matches the plain label text.
    expect(builtinOption.keywords).toEqual(['General']);
    const { container } = render(<>{builtinOption.label}</>);
    expect(container.textContent).toContain('General');
    expect(container.textContent).toContain(
      'knowledgeConfiguration.builtInSuffix',
    );
  });

  it('decodes a builtin selection into kind + rawId', () => {
    const onChange = jest.fn();
    render(<ParserSelect value={undefined} onChange={onChange} />);
    mockSelectProps.onChange('builtin:general');
    expect(onChange).toHaveBeenCalledWith('builtin', 'general');
  });

  it('decodes a pipeline selection into kind + rawId', () => {
    const onChange = jest.fn();
    render(<ParserSelect value={undefined} onChange={onChange} />);
    mockSelectProps.onChange('pipeline:pipe-1');
    expect(onChange).toHaveBeenCalledWith('pipeline', 'pipe-1');
  });

  it('emits null for an unrecognized/empty selection', () => {
    const onChange = jest.fn();
    render(<ParserSelect value={undefined} onChange={onChange} />);
    mockSelectProps.onChange('garbage');
    expect(onChange).toHaveBeenCalledWith(null, '');
  });

  it('forwards the loading flag from the options hook', () => {
    const parserOptions = jest.requireMock('@/hooks/use-parser-options');
    parserOptions.__setLoading(true);
    render(<ParserSelect value={undefined} onChange={jest.fn()} />);
    expect(mockSelectProps.loading).toBe(true);
    parserOptions.__setLoading(false);
  });
});
