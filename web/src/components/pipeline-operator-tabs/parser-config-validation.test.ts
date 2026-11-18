import i18n from '@/locales/config';
import { z } from 'zod';
import { addParserConfigIssues } from './parser-config-validation';

// The operator form schemas reach @/routes via navigate-hooks, which builds a
// browser router at import time and crashes outside a browser.
jest.mock('@/routes', () => ({ Routes: {} }));

type CollectedIssue = {
  path: (string | number)[];
  message?: string;
  code?: string;
};

function collectIssues(parserConfig: Record<string, any> | undefined) {
  const issues: CollectedIssue[] = [];
  const ctx = {
    addIssue: (issue: CollectedIssue) => {
      issues.push(issue);
    },
  };
  addParserConfigIssues(parserConfig, ctx as never, i18n.t);
  return issues;
}

describe('addParserConfigIssues: extractor toggles', () => {
  it('re-paths an enabled keywords group with top_n 0 onto parser_config', () => {
    const issues = collectIssues({
      'Extractor:abc': { keywords: { enabled: true, top_n: 0 } },
    });
    expect(issues).toEqual([
      {
        path: ['parser_config', 'Extractor:abc', 'keywords', 'top_n'],
        message: i18n.t('flow.extractorKeywordsTopNMissing'),
        code: z.ZodIssueCode.custom,
      },
    ]);
  });

  it('flags a tags group without a tag file', () => {
    const issues = collectIssues({
      'Extractor:abc': { tags: { enabled: true, top_n: 3 } },
    });
    expect(issues.map((x) => x.path.join('.'))).toEqual([
      'parser_config.Extractor:abc.tags.tag_file_id',
    ]);
  });

  it('accepts the untouched defaults (every group switched off)', () => {
    expect(collectIssues({ 'Extractor:abc': {} })).toEqual([]);
  });
});
