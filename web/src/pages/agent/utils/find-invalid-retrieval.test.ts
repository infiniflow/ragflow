import { findInvalidRetrievalBinding } from './find-invalid-retrieval';

const agentNode = (form: Record<string, any>, id = 'agent-1') => ({
  id,
  data: { label: 'Agent', name: 'Docs QA Agent', form },
});

describe('findInvalidRetrievalBinding', () => {
  it('does not flag a bound agent retrieval tool with stale template params', () => {
    // Template-era params: the dataset was bound at creation, but the dict
    // predates `rerank_candidates_count`. Saving must not report a missing
    // dataset for it.
    const nodes = [
      agentNode({
        llm_id: 'deepseek-chat',
        tools: [
          {
            component_name: 'Retrieval',
            params: {
              retrieval_from: 'dataset',
              dataset_ids: ['kb-1'],
              top_k: 1024,
            },
          },
        ],
      }),
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toBeUndefined();
  });

  it('flags an agent retrieval tool whose dataset binding is empty', () => {
    const nodes = [
      agentNode({
        llm_id: 'deepseek-chat',
        tools: [
          {
            component_name: 'Retrieval',
            params: {
              retrieval_from: 'dataset',
              dataset_ids: [],
              rerank_candidates_count: 64,
            },
          },
        ],
      }),
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toMatchObject({
      messageKey: 'flow.retrievalDatasetMissing',
    });
  });

  it('flags a retrieval node sourcing from memories without memories', () => {
    const nodes = [
      {
        id: 'retrieval-1',
        data: {
          label: 'Retrieval',
          name: 'Retrieval',
          form: {
            retrieval_from: 'memory',
            memory_ids: [],
            dataset_ids: [],
          },
        },
      },
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toMatchObject({
      messageKey: 'flow.retrievalMemoryMissing',
    });
  });

  it('flags a retrieval node sourcing from datasets without datasets', () => {
    const nodes = [
      {
        id: 'retrieval-1',
        data: {
          label: 'Retrieval',
          name: 'Retrieval',
          form: { retrieval_from: 'dataset', dataset_ids: [] },
        },
      },
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toMatchObject({
      messageKey: 'flow.retrievalDatasetMissing',
    });
  });

  it('keeps legacy kb_ids-only params out of the check', () => {
    const nodes = [
      agentNode({
        llm_id: 'deepseek-chat',
        tools: [
          {
            component_name: 'Retrieval',
            params: { retrieval_from: 'dataset', kb_ids: [], top_k: 1024 },
          },
        ],
      }),
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toBeUndefined();
  });

  it('accepts variable references as a dataset binding', () => {
    const nodes = [
      {
        id: 'retrieval-1',
        data: {
          label: 'Retrieval',
          name: 'Retrieval',
          form: { retrieval_from: 'dataset', dataset_ids: ['{begin@kb}'] },
        },
      },
    ] as any;

    expect(findInvalidRetrievalBinding(nodes)).toBeUndefined();
  });
});
