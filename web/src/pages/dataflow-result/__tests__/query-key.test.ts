import { buildPipelineFileLogDetailQueryKey } from '../query-key';

describe('pipeline file log detail query key', () => {
  it('separates knowledge and log identities and preserves refreshes', () => {
    expect(
      buildPipelineFileLogDetailQueryKey({
        knowledgeId: 'knowledge-a',
        logId: 'log-a',
        refreshCount: 2,
      }),
    ).toEqual(['fetchLogDetail', 'knowledge-a', 'log-a', 2]);
  });

  it('uses a disabled-ready key when identifiers are absent', () => {
    expect(
      buildPipelineFileLogDetailQueryKey({ knowledgeId: '', logId: undefined }),
    ).toEqual(['fetchLogDetail', '', '', 0]);
  });
});
