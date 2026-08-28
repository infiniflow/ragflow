import { RetrievalFrom } from '../../constant';

export function hasValidMemorySelection(
  retrievalFrom: string | undefined,
  memoryIds: string[] | undefined,
) {
  return retrievalFrom !== RetrievalFrom.Memory || Boolean(memoryIds?.length);
}
