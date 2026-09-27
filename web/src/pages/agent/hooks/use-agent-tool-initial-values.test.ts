import { getQueritAgentInitialValues } from './querit-agent-initial-values';

describe('getQueritAgentInitialValues', () => {
  it('creates Querit Agent params without runtime query or outputs', () => {
    expect(
      getQueritAgentInitialValues({
        api_key: '',
        query: 'sys.query',
        count: 10,
        chunks_per_doc: 3,
        site_include: [],
        site_exclude: [],
        time_range: '',
        country_include: [],
        language_include: [],
        outputs: {
          formalized_content: { value: '', type: 'string' },
          json: { value: {}, type: 'object' },
        },
      }),
    ).toEqual({
      api_key: '',
      count: 10,
      chunks_per_doc: 3,
      site_include: [],
      site_exclude: [],
      time_range: '',
      country_include: [],
      language_include: [],
    });
  });
});
