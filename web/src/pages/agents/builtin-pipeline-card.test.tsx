import { render, screen, fireEvent } from '@testing-library/react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { BuiltinPipelineCard } from './builtin-pipeline-card';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) =>
      key === 'knowledgeConfiguration.builtInSuffix'
        ? ' (built in)'
        : key === 'common.copy'
          ? 'Copy'
          : key,
  }),
}));

const mockCopy = jest.fn();
// Mutable so a test can flip the copying state before rendering.
var copyState = { copying: false };
jest.mock('./use-copy-builtin-pipeline', () => ({
  useCopyBuiltinPipeline: () => ({
    copy: mockCopy,
    copying: copyState.copying,
  }),
}));

const baseData: any = {
  id: 'general',
  title: 'General',
  description: 'Default parsing method',
  filename: 'ingestion_pipeline_general.json',
  canvas_category: 'dataflow_canvas',
  type: 'builtin_pipeline',
  builtin: true,
};

beforeEach(() => {
  mockCopy.mockClear();
  copyState.copying = false;
});

describe('BuiltinPipelineCard', () => {
  it('renders the title, description and a Built-in badge', () => {
    render(
      <TooltipProvider>
        <BuiltinPipelineCard data={baseData} />
      </TooltipProvider>,
    );
    expect(screen.getByText('General')).toBeInTheDocument();
    expect(screen.getByText('Default parsing method')).toBeInTheDocument();
    expect(screen.getByTestId('builtin-badge')).toHaveTextContent('built in');
  });

  it('renders a Copy button and no edit/delete dropdown', () => {
    render(
      <TooltipProvider>
        <BuiltinPipelineCard data={baseData} />
      </TooltipProvider>,
    );
    const copyButton = screen.getByTestId('copy-builtin-pipeline');
    expect(copyButton).toBeInTheDocument();
    expect(copyButton).toHaveTextContent('Copy');
    // Read-only: there is no "more" menu for built-in items.
    expect(screen.queryByText('common.rename')).not.toBeInTheDocument();
  });

  it('calls copy with the builtin id and title', () => {
    render(
      <TooltipProvider>
        <BuiltinPipelineCard data={baseData} />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId('copy-builtin-pipeline'));
    expect(mockCopy).toHaveBeenCalledWith({ id: 'general', title: 'General' });
  });

  it('is read-only: clicking the card body does not trigger copy', () => {
    render(
      <TooltipProvider>
        <BuiltinPipelineCard data={baseData} />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByTestId('builtin-pipeline-card'));
    expect(mockCopy).not.toHaveBeenCalled();
  });

  it('disables the Copy button while copying', () => {
    copyState.copying = true;
    render(
      <TooltipProvider>
        <BuiltinPipelineCard data={baseData} />
      </TooltipProvider>,
    );
    expect(screen.getByTestId('copy-builtin-pipeline')).toBeDisabled();
  });

  it('falls back gracefully when title or description is missing', () => {
    render(
      <TooltipProvider>
        <BuiltinPipelineCard
          data={{ ...baseData, title: undefined, description: undefined }}
        />
      </TooltipProvider>,
    );
    expect(screen.getByTestId('builtin-pipeline-card')).toBeInTheDocument();
    expect(screen.getByTestId('builtin-badge')).toBeInTheDocument();
  });
});
