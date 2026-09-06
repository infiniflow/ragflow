import { IDataset } from '@/interfaces/database/dataset';
import { hasCompilerOperatorConfig } from '@/utils/pipeline-operator';

export function canGenerateWiki(knowledgeBase?: IDataset): boolean {
  if ((knowledgeBase?.chunk_count ?? 0) <= 0) {
    return false;
  }
  if (knowledgeBase?.pipeline_id?.trim()) {
    return true;
  }
  const parserConfig = knowledgeBase?.parser_config as
    | Record<string, unknown>
    | undefined;
  return hasCompilerOperatorConfig(parserConfig);
}
