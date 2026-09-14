export type ChatChannelTargetType = 'chat' | 'agent';

export interface ChatChannelBinding {
  chat_id: string | null;
  agent_id: string | null;
}

export const toChatChannelTargetValue = (
  targetType?: ChatChannelTargetType,
  targetId?: string | null,
) => (targetType && targetId ? `${targetType}:${targetId}` : undefined);

export const toChatChannelBinding = (value?: string): ChatChannelBinding => {
  if (!value) {
    return { chat_id: null, agent_id: null };
  }

  const separator = value.indexOf(':');
  const type = value.slice(0, separator);
  const id = separator >= 0 ? value.slice(separator + 1) : '';
  if (type === 'agent' && id) {
    return { chat_id: null, agent_id: id };
  }
  if (type === 'chat' && id) {
    return { chat_id: id, agent_id: null };
  }
  return { chat_id: null, agent_id: null };
};
