import { render, screen } from '@testing-library/react';
import { MetadataFilterDiagnostics } from './metadata-filter-diagnostics';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: { count?: number }) => {
      const translations: Record<string, string> = {
        'chat.metadataFilter': 'Metadata filter',
        'chat.metadataFilterApplied': 'Applied',
        'chat.metadataFilterNoMatches': 'No matches',
        'chat.metadataFilterUnavailable': 'Filter information is unavailable',
        'chat.metadataFilterMatchedDocuments': `Matched documents: ${params?.count}`,
      };
      return translations[key] ?? key;
    },
  }),
}));

describe('MetadataFilterDiagnostics', () => {
  it('renders applied filter conditions and match count', () => {
    render(
      <MetadataFilterDiagnostics
        diagnostics={[
          {
            method: 'semi_auto',
            status: 'applied',
            conditions: [
              { key: 'project', op: 'contains', value: '2018_055 NST' },
              { key: 'phase', op: 'in', value: ['SP'] },
            ],
            logic: 'and',
            matched_document_count: 71700,
            tool_name: 'search_archa_data',
          },
        ]}
      />,
    );

    expect(screen.getByText('Metadata filter')).toBeInTheDocument();
    expect(screen.getByText('Applied').parentElement).toHaveTextContent(
      'search_archa_data: Applied',
    );
    expect(
      screen.getByText('project contains 2018_055 NST'),
    ).toBeInTheDocument();
    expect(screen.getByText('phase in ["SP"]')).toBeInTheDocument();
    expect(screen.getByText('Matched documents: 71700')).toBeInTheDocument();
  });

  it('renders multiple retrieval diagnostics', () => {
    render(
      <MetadataFilterDiagnostics
        diagnostics={[
          {
            method: 'semi_auto',
            status: 'applied',
            tool_name: 'search_archa_data',
          },
          {
            method: 'semi_auto',
            status: 'no_matches',
            tool_name: 'search_archa_metodika',
          },
        ]}
      />,
    );

    expect(screen.getByText('Applied').parentElement).toHaveTextContent(
      'search_archa_data: Applied',
    );
    expect(screen.getByText('No matches').parentElement).toHaveTextContent(
      'search_archa_metodika: No matches',
    );
  });

  it.each([undefined, []])(
    'renders the fallback for unavailable diagnostics',
    (diagnostics) => {
      render(<MetadataFilterDiagnostics diagnostics={diagnostics} />);

      expect(
        screen.getByText('Filter information is unavailable'),
      ).toBeInTheDocument();
    },
  );
});
