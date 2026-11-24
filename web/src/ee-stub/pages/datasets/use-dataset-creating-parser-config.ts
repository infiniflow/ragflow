import type { ParseType } from '@/constants/knowledge';

type DatasetCreatingParserConfigParams = {
  parseType?: ParseType;
  parserId?: string;
  pipelineId?: string;
  pipelineVersionId?: string;
};

export function useDatasetCreatingParserConfig(
  _params: DatasetCreatingParserConfigParams,
): { parserConfig?: Record<string, any>; loading: boolean } {
  void _params;
  return { loading: false };
}
