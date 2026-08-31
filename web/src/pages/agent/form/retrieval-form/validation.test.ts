import { RetrievalFrom } from '../../constant';
import { hasValidMemorySelection } from './validation';

describe('hasValidMemorySelection', () => {
  it.each([
    [RetrievalFrom.Dataset, undefined, true],
    [RetrievalFrom.Dataset, [], true],
    [RetrievalFrom.Memory, ['memory-id'], true],
    [RetrievalFrom.Memory, [], false],
  ])('validates %p with %p as %p', (retrievalFrom, memoryIds, expected) => {
    expect(hasValidMemorySelection(retrievalFrom, memoryIds)).toBe(expected);
  });
});
