import { IDataset } from '@/interfaces/database/dataset';

type CompilationParserConfig = IDataset['parser_config'] & {
  compilation_template_group_id?: string | string[];
  compilation_template_group_ids?: string | string[];
};

function hasConfiguredGroup(value: unknown): boolean {
  if (Array.isArray(value)) {
    return value.some(
      (groupId) => typeof groupId === 'string' && groupId.trim().length > 0,
    );
  }

  return typeof value === 'string' && value.trim().length > 0;
}

export function canGenerateWiki(knowledgeBase?: IDataset): boolean {
  if ((knowledgeBase?.chunk_count ?? 0) <= 0) {
    return false;
  }

  const parserConfig = knowledgeBase?.parser_config as
    | CompilationParserConfig
    | undefined;
  const compilationGroupIds =
    parserConfig?.compilation_template_group_ids ??
    parserConfig?.compilation_template_group_id;

  return (
    Boolean(knowledgeBase?.pipeline_id?.trim()) ||
    hasConfiguredGroup(compilationGroupIds)
  );
}
