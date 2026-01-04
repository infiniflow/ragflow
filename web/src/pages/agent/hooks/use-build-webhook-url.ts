import { useParams } from 'react-router';

export function useBuildWebhookUrl() {
  const { id } = useParams();

  const text = `${location.protocol}//${location.host}/api/v1/webhook/${id}`;
  return text;
}
