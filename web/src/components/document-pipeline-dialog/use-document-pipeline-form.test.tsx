import { renderHook } from '@testing-library/react';
import { useDocumentPipelineForm } from './use-document-pipeline-form';

jest.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
jest.mock('@/utils/backend-runtime', () => ({
  getBackendLanguage: () => 'go',
}));
jest.mock('@/hooks/use-pipeline-operator', () => ({
  useActiveTab: () => ({ activeTab: '', setActiveTab: jest.fn() }),
  usePipelineOperatorNodes: () => ({ operatorNodes: [], loading: false }),
  useResetParserConfigOnPipelineChange: jest.fn(),
}));
// The validator is only reached through the submit-time schema, and its module
// pulls the whole agent form graph in.
jest.mock('../pipeline-operator-tabs/parser-config-validation', () => ({
  addParserConfigIssues: jest.fn(),
}));

describe('useDocumentPipelineForm buildSubmitData', () => {
  // The request interface names the fixed keys only; what a parser writes into
  // parser_config is an open record.
  const buildParserConfig = (
    parser_config: Record<string, any>,
    parseType = 'built-in',
  ) => {
    const { result } = renderHook(() =>
      useDocumentPipelineForm({
        parserId: 'table',
        pipelineId: parseType === 'pipeline' ? 'pipeline-1' : undefined,
        parserConfig: {},
      }),
    );
    return result.current.buildSubmitData({
      parseType,
      parser_id: 'table',
      pipeline_id: parseType === 'pipeline' ? 'pipeline-1' : '',
      parser_config,
    } as never).parser_config as Record<string, any>;
  };

  // Ingestion reads the user's column intent from the root keys, so a document
  // that changes it here has to be seen by the next parse.
  it('writes the manual profile the form states to the root keys', () => {
    const config = buildParserConfig({
      'Parser:HipSignsRhyme': {
        setups: [
          {
            fileFormat: 'spreadsheet',
            column_mode: 'manual',
            column_roles: { Name: 'metadata' },
            column_names: ['Name'],
          },
        ],
      },
    });

    expect(config.table_column_mode).toBe('manual');
    expect(config.table_column_roles).toEqual({ Name: 'metadata' });
    expect(config['Parser:HipSignsRhyme'].spreadsheet).toEqual(
      expect.objectContaining({
        column_mode: 'manual',
        column_roles: { Name: 'metadata' },
      }),
    );
  });

  it('states no column intent for a parser that configures none', () => {
    const config = buildParserConfig({
      'Parser:HipSignsRhyme': {
        setups: [{ fileFormat: 'pdf', parse_method: 'DeepDOC' }],
      },
    });

    expect(config).not.toHaveProperty('table_column_mode');
    expect(config).not.toHaveProperty('table_column_roles');
  });
});

describe('useDocumentPipelineForm isTableParser', () => {
  const render = (parserId: string, pipelineId?: string) => {
    const { result } = renderHook(() =>
      useDocumentPipelineForm({ parserId, pipelineId, parserConfig: {} }),
    );
    return result.current.isTableParser;
  };

  // The column controls belong to the table parser: a built-in document that
  // parses another way must not offer a setting its parser ignores.
  it('shows column settings for a table document', () => {
    expect(render('table')).toBe(true);
  });

  it('hides column settings for a built-in document that is not a table', () => {
    expect(render('naive')).toBe(false);
  });

  // A canvas parser decides which file types it reads, not the document's chunk
  // method, so its dialog keeps the controls whatever the document is typed as.
  it('shows column settings for a pipeline document', () => {
    expect(render('naive', 'pipeline-1')).toBe(true);
  });
});
