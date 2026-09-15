import {
  bindUnboundRetrieval,
  countUnboundRetrieval,
} from './template-retrieval-binding';

// Mirrors the "Your starter dataset chatbot" template shape: the Retrieval
// tool is embedded in an Agent node and appears in BOTH DSL views.
const starterLikeDsl = {
  graph: {
    nodes: [
      {
        id: 'Agent:1',
        data: {
          label: 'Agent',
          form: {
            llm_id: '',
            tools: [
              {
                component_name: 'Retrieval',
                params: {
                  retrieval_from: 'dataset',
                  kb_ids: [],
                  top_k: 1024,
                },
              },
            ],
          },
        },
      },
    ],
  },
  components: {
    'Agent:1': {
      obj: {
        component_name: 'Agent',
        params: {
          llm_id: '',
          tools: [
            {
              component_name: 'Retrieval',
              params: {
                retrieval_from: 'dataset',
                kb_ids: [],
                top_k: 1024,
              },
            },
          ],
        },
      },
    },
  },
};

describe('countUnboundRetrieval', () => {
  it('counts every unbound dataset retrieval in both DSL views', () => {
    expect(countUnboundRetrieval(starterLikeDsl as any)).toEqual({
      datasetCount: 2,
      memoryCount: 0,
    });
  });

  it('ignores already-bound steps and legacy-free DSLs', () => {
    const bound = bindUnboundRetrieval(
      starterLikeDsl as any,
      ['kb-1'],
      [],
    ) as any;
    expect(countUnboundRetrieval(bound)).toEqual({
      datasetCount: 0,
      memoryCount: 0,
    });
  });

  it('returns zeroes for a missing DSL', () => {
    expect(countUnboundRetrieval(undefined)).toEqual({
      datasetCount: 0,
      memoryCount: 0,
    });
  });
});

describe('bindUnboundRetrieval', () => {
  it('binds the selected dataset to both views and drops legacy kb_ids', () => {
    const bound = bindUnboundRetrieval(
      starterLikeDsl as any,
      ['kb-1'],
      [],
    ) as any;

    const graphToolParams = bound.graph.nodes[0].data.form.tools[0]
      .params as Record<string, any>;
    const componentToolParams = (
      bound.components['Agent:1'].obj.params.tools[0] as any
    ).params as Record<string, any>;

    expect(graphToolParams.dataset_ids).toEqual(['kb-1']);
    expect(graphToolParams.kb_ids).toBeUndefined();
    expect(componentToolParams.dataset_ids).toEqual(['kb-1']);
    expect(componentToolParams.kb_ids).toBeUndefined();
  });

  it('does not mutate the input DSL', () => {
    bindUnboundRetrieval(starterLikeDsl as any, ['kb-1'], []);
    const params = starterLikeDsl.graph.nodes[0].data.form.tools[0]
      .params as Record<string, any>;
    expect(params.dataset_ids).toBeUndefined();
  });

  it('leaves steps that already carry ids untouched', () => {
    const dsl = {
      graph: {
        nodes: [
          {
            id: 'r1',
            data: {
              label: 'Retrieval',
              form: { retrieval_from: 'dataset', dataset_ids: ['kb-x'] },
            },
          },
        ],
      },
    };
    const bound = bindUnboundRetrieval(dsl as any, ['kb-1'], []) as any;
    expect(
      (bound.graph.nodes[0].data.form as Record<string, any>).dataset_ids,
    ).toEqual(['kb-x']);
  });
});
