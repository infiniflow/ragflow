import { AgentDialogueMode, BeginId } from '../constant';
import useGraphStore from '../store';

export function useIsWebhookMode() {
  const getNode = useGraphStore((state) => state.getNode);

  const beginNode = getNode(BeginId);

  return beginNode?.data.form?.mode === AgentDialogueMode.Webhook;
}
