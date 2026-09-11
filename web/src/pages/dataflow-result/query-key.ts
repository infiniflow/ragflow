export const buildPipelineFileLogDetailQueryKey = ({
  knowledgeId,
  logId,
  refreshCount,
}: {
  knowledgeId: string;
  logId?: string;
  refreshCount?: number;
}): (string | number)[] => [
  'fetchLogDetail',
  knowledgeId,
  logId || '',
  typeof refreshCount === 'number' ? refreshCount : 0,
];
