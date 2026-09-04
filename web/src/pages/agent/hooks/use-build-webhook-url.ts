import { useParams } from 'react-router';
import { withAppBasePath } from '@/utils/base-path';

export function useBuildWebhookUrl() {
  const { id } = useParams();

  const text = `${location.origin}${withAppBasePath(`/api/v1/agents/${id}/webhook`)}`;
  return text;
}
