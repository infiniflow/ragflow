import {
  QueritFormSchema,
  deserializeQueritFormValues,
  serializeQueritFormValues,
  validateQueritFormValuesForPersistence,
} from './utils';

describe('Querit form contract', () => {
  const validValues = {
    api_key: '',
    query: 'begin.query',
    count: 10,
    chunks_per_doc: 3,
    site_include: [],
    site_exclude: [],
    time_range: '',
    country_include: [],
    language_include: [],
  };

  it.each(['d7', 'w2', 'm1', 'y1', '2026-01-01to2026-01-31', ''])(
    'accepts the supported time range %p',
    (timeRange) => {
      expect(
        QueritFormSchema.safeParse({
          ...validValues,
          time_range: timeRange,
        }).success,
      ).toBe(true);
    },
  );

  it.each(['d0', '7d', '2026-01-01-2026-01-31', 'last week'])(
    'rejects the unsupported time range %p',
    (timeRange) => {
      expect(
        QueritFormSchema.safeParse({
          ...validValues,
          time_range: timeRange,
        }).success,
      ).toBe(false);
    },
  );

  it.each([0, -1, 1.5])('rejects the invalid result count %p', (count) => {
    expect(
      QueritFormSchema.safeParse({
        ...validValues,
        count,
      }).success,
    ).toBe(false);
  });

  it.each([0, 1.5, 4])(
    'rejects the invalid chunks per document value %p',
    (chunksPerDoc) => {
      expect(
        QueritFormSchema.safeParse({
          ...validValues,
          chunks_per_doc: chunksPerDoc,
        }).success,
      ).toBe(false);
    },
  );

  it.each(['', '   '])('rejects an empty dynamic list item %p', (listItem) => {
    expect(
      QueritFormSchema.safeParse({
        ...validValues,
        site_include: [{ value: listItem }],
      }).success,
    ).toBe(false);
  });

  it.each(['', '   '])('rejects an empty query %p', (query) => {
    expect(
      QueritFormSchema.safeParse({
        ...validValues,
        query,
      }).success,
    ).toBe(false);
  });
});

describe('Querit form persistence', () => {
  it('rehydrates persisted string arrays for dynamic form fields', () => {
    expect(
      deserializeQueritFormValues({
        api_key: '',
        query: 'begin.query',
        count: 5,
        chunks_per_doc: 2,
        site_include: ['docs.example.com'],
        site_exclude: ['spam.example.com'],
        time_range: 'w1',
        country_include: ['united states'],
        language_include: ['en'],
      }),
    ).toMatchObject({
      site_include: [{ value: 'docs.example.com' }],
      site_exclude: [{ value: 'spam.example.com' }],
      country_include: [{ value: 'united states' }],
      language_include: [{ value: 'en' }],
    });
  });

  it('serializes dynamic form fields as string arrays', () => {
    expect(
      serializeQueritFormValues({
        api_key: '',
        query: 'begin.query',
        count: 5,
        chunks_per_doc: 2,
        site_include: [{ value: 'docs.example.com' }],
        site_exclude: [{ value: 'spam.example.com' }],
        time_range: 'w1',
        country_include: [{ value: 'united states' }],
        language_include: [{ value: 'en' }],
      }),
    ).toEqual({
      api_key: '',
      query: 'begin.query',
      count: 5,
      chunks_per_doc: 2,
      site_include: ['docs.example.com'],
      site_exclude: ['spam.example.com'],
      time_range: 'w1',
      country_include: ['united states'],
      language_include: ['en'],
    });
  });

  it('serializes valid numeric input values as numbers', () => {
    expect(
      serializeQueritFormValues({
        api_key: '',
        query: 'begin.query',
        count: '5',
        chunks_per_doc: '2',
        site_include: [],
        site_exclude: [],
        time_range: '',
        country_include: [],
        language_include: [],
      }),
    ).toMatchObject({
      count: 5,
      chunks_per_doc: 2,
    });
  });

  it('does not produce persisted values when the current form is invalid', () => {
    expect(
      validateQueritFormValuesForPersistence({
        api_key: '',
        query: 'begin.query',
        count: 0,
        chunks_per_doc: 3,
        site_include: [],
        site_exclude: [],
        time_range: '',
        country_include: [],
        language_include: [],
      }),
    ).toBeUndefined();
  });

  it('produces normalized persisted values when the current form is valid', () => {
    expect(
      validateQueritFormValuesForPersistence({
        api_key: '',
        query: 'begin.query',
        count: '5',
        chunks_per_doc: '2',
        site_include: [{ value: 'docs.example.com' }],
        site_exclude: [],
        time_range: 'w1',
        country_include: [],
        language_include: [{ value: 'en' }],
      }),
    ).toMatchObject({
      count: 5,
      chunks_per_doc: 2,
      site_include: ['docs.example.com'],
      language_include: ['en'],
    });
  });

  it('trims dynamic list values before persistence', () => {
    expect(
      validateQueritFormValuesForPersistence({
        api_key: '',
        query: 'begin.query',
        count: 5,
        chunks_per_doc: 2,
        site_include: [{ value: '  docs.example.com  ' }],
        site_exclude: [],
        time_range: '',
        country_include: [],
        language_include: [{ value: ' en ' }],
      }),
    ).toMatchObject({
      site_include: ['docs.example.com'],
      language_include: ['en'],
    });
  });

  it('normalizes missing persisted list fields to empty arrays', () => {
    expect(
      deserializeQueritFormValues({
        api_key: '',
        query: 'begin.query',
        count: 10,
        chunks_per_doc: 3,
        time_range: '',
      }),
    ).toMatchObject({
      site_include: [],
      site_exclude: [],
      country_include: [],
      language_include: [],
    });
  });
});
