import { chatPromptKbIssues } from './validate-chat-prompt';

const t = (key: string) => key;

describe('chatPromptKbIssues', () => {
  it('flags a system prompt missing {knowledge} when datasets are selected', () => {
    const issues = chatPromptKbIssues(
      { dataset_ids: ['kb1'], prompt_config: { system: 'answer the question' } },
      t,
    );
    expect(issues).toContainEqual({
      path: ['prompt_config', 'system'],
      message: 'knowledgePlaceholderMessage',
    });
  });

  it('does not flag when {knowledge} is present in the system prompt', () => {
    const issues = chatPromptKbIssues(
      {
        dataset_ids: ['kb1'],
        prompt_config: { system: 'answer using {knowledge}' },
      },
      t,
    );
    expect(issues).toEqual([]);
  });

  it('does not flag an empty system prompt (min(1) covers that instead)', () => {
    const issues = chatPromptKbIssues(
      { dataset_ids: ['kb1'], prompt_config: { system: '' } },
      t,
    );
    expect(issues).toEqual([]);
  });

  it('flags a non-empty empty_response when no dataset is selected', () => {
    const issues = chatPromptKbIssues(
      {
        dataset_ids: [],
        prompt_config: { system: '{knowledge}', empty_response: 'nothing found' },
      },
      t,
    );
    expect(issues).toContainEqual({
      path: ['prompt_config', 'empty_response'],
      message: 'emptyResponseMessage',
    });
  });

  it('does not flag an empty empty_response when no dataset is selected', () => {
    const issues = chatPromptKbIssues(
      { dataset_ids: [], prompt_config: { system: 'x', empty_response: '' } },
      t,
    );
    expect(issues).toEqual([]);
  });

  it('does not flag empty_response when datasets are selected', () => {
    const issues = chatPromptKbIssues(
      {
        dataset_ids: ['kb1'],
        prompt_config: { system: '{knowledge}', empty_response: 'none' },
      },
      t,
    );
    expect(issues).toEqual([]);
  });

  it('ignores a whitespace-only empty_response', () => {
    const issues = chatPromptKbIssues(
      {
        dataset_ids: [],
        prompt_config: { system: 'x', empty_response: '   ' },
      },
      t,
    );
    expect(issues).toEqual([]);
  });
});
