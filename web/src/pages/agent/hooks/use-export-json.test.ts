import { Operator } from '@/constants/agent';
import { clearSensitiveFields } from './clear-sensitive-fields';

describe('clearSensitiveFields', () => {
  it('clears a Querit API key without removing non-sensitive params', () => {
    const dsl = {
      tools: [
        {
          component_name: Operator.QueritSearch,
          params: {
            api_key: 'querit-secret',
            count: 5,
            chunks_per_doc: 2,
            site_include: ['docs.example.com'],
          },
        },
      ],
    };

    expect(clearSensitiveFields(dsl)).toEqual({
      tools: [
        {
          component_name: Operator.QueritSearch,
          params: {
            api_key: '',
            count: 5,
            chunks_per_doc: 2,
            site_include: ['docs.example.com'],
          },
        },
      ],
    });
    expect(dsl.tools[0].params.api_key).toBe('querit-secret');
  });
});
