import {
  bindUnboundRetrieval,
  collectUnboundRetrievalBindings,
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
                id: 'Retrieval:WarmWeeksRead',
                name: 'Retrieval',
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
              id: 'Retrieval:WarmOwlsCough',
              name: 'Retrieval',
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

describe('collectUnboundRetrievalBindings', () => {
  it('counts a retrieval mirrored across both DSL views once', () => {
    expect(collectUnboundRetrievalBindings(starterLikeDsl as any)).toEqual({
      datasetBlocks: [
        {
          blockId: 'Retrieval:WarmWeeksRead',
          displayName: 'Retrieval',
        },
      ],
      memoryCount: 0,
    });
  });

  it('ignores already-bound steps and legacy-free DSLs', () => {
    const bound = bindUnboundRetrieval(
      starterLikeDsl as any,
      { 'Retrieval:WarmWeeksRead': ['kb-1'] },
      [],
    ) as any;
    expect(collectUnboundRetrievalBindings(bound)).toEqual({
      datasetBlocks: [],
      memoryCount: 0,
    });
  });

  it('returns zeroes for a missing DSL', () => {
    expect(collectUnboundRetrievalBindings(undefined)).toEqual({
      datasetBlocks: [],
      memoryCount: 0,
    });
  });

  it('returns stable block ids and graph display names', () => {
    const dsl = standaloneRetrievalDsl(['Schema', 'Examples', 'Description']);

    expect(collectUnboundRetrievalBindings(dsl as any)).toEqual({
      datasetBlocks: [
        { blockId: 'retrieval-0', displayName: 'Schema' },
        { blockId: 'retrieval-1', displayName: 'Examples' },
        { blockId: 'retrieval-2', displayName: 'Description' },
      ],
      memoryCount: 0,
    });
  });
});

describe('bindUnboundRetrieval', () => {
  it('binds the selected dataset to both views and drops legacy kb_ids', () => {
    const bound = bindUnboundRetrieval(
      starterLikeDsl as any,
      { 'Retrieval:WarmWeeksRead': ['kb-1'] },
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
    bindUnboundRetrieval(
      starterLikeDsl as any,
      { 'Retrieval:WarmWeeksRead': ['kb-1'] },
      [],
    );
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
    const bound = bindUnboundRetrieval(dsl as any, { r1: ['kb-1'] }, []) as any;
    expect(
      (bound.graph.nodes[0].data.form as Record<string, any>).dataset_ids,
    ).toEqual(['kb-x']);
  });

  it.each([
    ['different datasets', [['kb-1'], ['kb-2']]],
    ['the same dataset in two blocks', [['kb-1'], ['kb-1']]],
    ['the same dataset in three blocks', [['kb-1'], ['kb-1'], ['kb-1']]],
  ])('preserves %s', (_name, selections) => {
    const dsl = standaloneRetrievalDsl(
      selections.map((_, index) => `Block ${index + 1}`),
    );
    const bindings = Object.fromEntries(
      selections.map((datasetIds, index) => [`retrieval-${index}`, datasetIds]),
    );

    const bound = bindUnboundRetrieval(dsl as any, bindings, []) as any;

    expect(
      bound.graph.nodes.map((node: any) => node.data.form.dataset_ids),
    ).toEqual(selections);
    expect(
      Object.values(bound.components).map(
        (component: any) => component.obj.params.dataset_ids,
      ),
    ).toEqual(selections);
  });

  it('pairs embedded retrieval mirrors by owner and tool position', () => {
    const dsl = {
      graph: {
        nodes: [
          {
            id: 'agent-1',
            data: {
              label: 'Agent',
              name: 'Research agent',
              form: {
                tools: [
                  {
                    id: 'graph-retrieval-id',
                    name: 'Research sources',
                    component_name: 'Retrieval',
                    params: { retrieval_from: 'dataset', dataset_ids: [] },
                  },
                ],
              },
            },
          },
        ],
      },
      components: {
        'agent-1': {
          obj: {
            component_name: 'Agent',
            params: {
              tools: [
                {
                  id: 'component-retrieval-id',
                  name: 'Retrieval',
                  component_name: 'Retrieval',
                  params: { retrieval_from: 'dataset', dataset_ids: [] },
                },
              ],
            },
          },
        },
      },
    };

    const bound = bindUnboundRetrieval(
      dsl as any,
      { 'graph-retrieval-id': ['kb-1'] },
      [],
    ) as any;

    expect(bound.graph.nodes[0].data.form.tools[0].params.dataset_ids).toEqual([
      'kb-1',
    ]);
    expect(
      bound.components['agent-1'].obj.params.tools[0].params.dataset_ids,
    ).toEqual(['kb-1']);
  });
});

function standaloneRetrievalDsl(names: string[]) {
  return {
    graph: {
      nodes: names.map((name, index) => ({
        id: `retrieval-${index}`,
        data: {
          label: 'Retrieval',
          name,
          form: { retrieval_from: 'dataset', dataset_ids: [] },
        },
      })),
    },
    components: Object.fromEntries(
      names.map((_, index) => [
        `retrieval-${index}`,
        {
          obj: {
            component_name: 'Retrieval',
            params: { retrieval_from: 'dataset', dataset_ids: [] },
          },
        },
      ]),
    ),
  };
}
