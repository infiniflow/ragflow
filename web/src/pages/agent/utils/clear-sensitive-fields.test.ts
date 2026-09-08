jest.mock('@/constants/agent', () => ({
  Operator: {
    TavilySearch: 'TavilySearch',
    TavilyExtract: 'TavilyExtract',
    Google: 'Google',
    KeenableSearch: 'KeenableSearch',
    YouComSearch: 'YouComSearch',
    BGPT: 'Bing',
    QueritContents: 'QueritContents',
    QueritSearch: 'QueritSearch',
  },
}));

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

  it.each([
    'querit',
    'querit_contents',
    'queritcontents',
    'querit_search',
    'queritsearch',
  ])('clears a Querit API key for the %s registry alias', (componentName) => {
    const dsl = {
      tools: [
        {
          component_name: componentName,
          params: {
            api_key: 'querit-secret',
            count: 5,
          },
        },
      ],
    };

    const sanitized = clearSensitiveFields(dsl);

    expect(sanitized.tools[0].params.api_key).toBe('');
    expect(sanitized.tools[0].params.count).toBe(5);
    expect(dsl.tools[0].params.api_key).toBe('querit-secret');
  });

  it('clears a standalone Querit Canvas key from graph and components', () => {
    const dsl = {
      graph: {
        nodes: [
          {
            data: {
              label: Operator.QueritSearch,
              form: {
                api_key: 'graph-secret',
                count: 5,
              },
            },
          },
        ],
      },
      components: {
        querit: {
          obj: {
            component_name: Operator.QueritSearch,
            params: {
              api_key: 'component-secret',
              count: 5,
            },
          },
        },
      },
    };

    const sanitized = clearSensitiveFields(dsl);

    expect(sanitized.graph.nodes[0].data.form.api_key).toBe('');
    expect(sanitized.graph.nodes[0].data.form.count).toBe(5);
    expect(sanitized.components.querit.obj.params.api_key).toBe('');
    expect(sanitized.components.querit.obj.params.count).toBe(5);
    expect(dsl.graph.nodes[0].data.form.api_key).toBe('graph-secret');
    expect(dsl.components.querit.obj.params.api_key).toBe('component-secret');
  });

  it('clears a You.com key from a canvas node and from a tool record', () => {
    const dsl = {
      graph: {
        nodes: [
          {
            data: {
              label: Operator.YouComSearch,
              form: {
                api_key: 'ydc-graph-secret',
                freshness: 'week',
              },
            },
          },
        ],
      },
      tools: [
        {
          component_name: Operator.YouComSearch,
          params: {
            api_key: 'ydc-tool-secret',
            top_n: 10,
          },
        },
      ],
    };

    const sanitized = clearSensitiveFields(dsl);

    expect(sanitized.graph.nodes[0].data.form.api_key).toBe('');
    expect(sanitized.graph.nodes[0].data.form.freshness).toBe('week');
    expect(sanitized.tools[0].params.api_key).toBe('');
    expect(sanitized.tools[0].params.top_n).toBe(10);
    expect(dsl.graph.nodes[0].data.form.api_key).toBe('ydc-graph-secret');
    expect(dsl.tools[0].params.api_key).toBe('ydc-tool-secret');
  });

  it('does not change standalone graph export behavior for other tools', () => {
    const dsl = {
      graph: {
        nodes: [
          {
            data: {
              label: Operator.TavilySearch,
              form: {
                api_key: 'existing-tavily-key',
              },
            },
          },
        ],
      },
    };

    expect(clearSensitiveFields(dsl)).toEqual(dsl);
  });
});
