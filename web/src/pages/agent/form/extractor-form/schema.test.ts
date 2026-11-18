import i18n from '@/locales/config';
import { FormSchema } from './schema';

// LlmSettingSchema's module chain reaches @/routes via navigate-hooks, which
// builds a browser router at import time and crashes outside a browser.
jest.mock('@/routes', () => ({ Routes: {} }));

const issuePaths = (data: Record<string, any>) => {
  const result = FormSchema.safeParse(data);
  return result.success
    ? []
    : result.error.issues.map((issue) => issue.path.join('.'));
};

describe('extractor FormSchema extraction toggles', () => {
  it('passes with every group switched off and top_n 0 (defaults)', () => {
    const result = FormSchema.safeParse({
      keywords: { enabled: false, top_n: 0 },
      questions: { enabled: false, top_n: 0 },
      tags: { enabled: false, top_n: 0, tag_file_id: '' },
    });
    expect(result.success).toBe(true);
  });

  it('requires keywords.top_n >= 1 when keywords is enabled', () => {
    expect(issuePaths({ keywords: { enabled: true, top_n: 0 } })).toContain(
      'keywords.top_n',
    );
    const result = FormSchema.safeParse({
      keywords: { enabled: true, top_n: 0 },
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0].message).toBe(
        i18n.t('flow.extractorKeywordsTopNMissing'),
      );
    }
  });

  it('treats numeric 1 as enabled for questions', () => {
    expect(issuePaths({ questions: { enabled: 1 } })).toContain(
      'questions.top_n',
    );
  });

  it('requires tags.top_n >= 1 and a tag file when tags is enabled', () => {
    expect(issuePaths({ tags: { enabled: true, top_n: 0 } })).toEqual(
      expect.arrayContaining(['tags.top_n', 'tags.tag_file_id']),
    );
    expect(issuePaths({ tags: { enabled: true, top_n: 3 } })).toEqual([
      'tags.tag_file_id',
    ]);
    expect(
      FormSchema.safeParse({
        tags: { enabled: true, top_n: 3, tag_file_id: 'tag-1' },
      }).success,
    ).toBe(true);
  });

  it('ignores legacy groups without an enabled key', () => {
    const result = FormSchema.safeParse({
      keywords: { top_n: 3 },
      questions: { top_n: 2 },
    });
    expect(result.success).toBe(true);
  });
});
