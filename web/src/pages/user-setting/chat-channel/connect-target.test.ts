import { toChatChannelBinding } from './connect-target';

describe('chat channel assistant binding', () => {
  it('binds an agent while clearing the ordinary chat assistant', () => {
    expect(toChatChannelBinding('agent:agent-1')).toEqual({
      chat_id: null,
      agent_id: 'agent-1',
    });
  });

  it('binds an ordinary chat assistant while clearing the agent', () => {
    expect(toChatChannelBinding('chat:chat-1')).toEqual({
      chat_id: 'chat-1',
      agent_id: null,
    });
  });

  it('clears both targets when the selection is removed', () => {
    expect(toChatChannelBinding(undefined)).toEqual({
      chat_id: null,
      agent_id: null,
    });
  });
});
