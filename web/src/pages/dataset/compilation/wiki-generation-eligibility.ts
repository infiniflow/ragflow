import { IDataset } from '@/interfaces/database/dataset';

export function canGenerateWiki(knowledgeBase?: IDataset): boolean {
  return (
    (knowledgeBase?.chunk_count ?? 0) > 0 &&
    Boolean(knowledgeBase?.pipeline_id?.trim())
  );
}
