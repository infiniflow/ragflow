import { ReactNode } from 'react';
import { useSearchParams } from 'react-router';

export function PipelinePreviewResult({ fallback }: { fallback: ReactNode }) {
  const [params] = useSearchParams();
  return params.has('agent_id') ? null : fallback;
}
