import { getKnowledgeFileParserId } from '../knowledge-file';

test('prefers the API chunk_method and keeps parser_id compatibility', () => {
  expect(
    getKnowledgeFileParserId({ parser_id: 'naive', chunk_method: 'tag' }),
  ).toBe('tag');
  expect(getKnowledgeFileParserId({ parser_id: 'naive' })).toBe('naive');
});
