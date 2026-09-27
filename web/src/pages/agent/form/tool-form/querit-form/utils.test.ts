import {
  deserializeQueritAgentFormValues,
  validateQueritAgentFormValuesForPersistence,
} from './utils';

describe('Querit Agent form persistence', () => {
  it('rehydrates persisted string arrays for the embedded form', () => {
    expect(
      deserializeQueritAgentFormValues({
        api_key: '',
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

  it('normalizes valid values without persisting a runtime query', () => {
    expect(
      validateQueritAgentFormValuesForPersistence({
        api_key: 'secret',
        query: 'must-not-be-persisted',
        count: '5',
        chunks_per_doc: '2',
        site_include: [{ value: ' docs.example.com ' }],
        site_exclude: [],
        time_range: 'w1',
        country_include: [{ value: ' united states ' }],
        language_include: [{ value: ' en ' }],
      }),
    ).toEqual({
      api_key: 'secret',
      count: 5,
      chunks_per_doc: 2,
      site_include: ['docs.example.com'],
      site_exclude: [],
      time_range: 'w1',
      country_include: ['united states'],
      language_include: ['en'],
    });
  });

  it('does not produce persisted values when the embedded form is invalid', () => {
    expect(
      validateQueritAgentFormValuesForPersistence({
        api_key: '',
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
});
