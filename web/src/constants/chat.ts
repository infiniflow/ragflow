export enum MessageType {
  Assistant = 'assistant',
  User = 'user',
}

export const variableEnabledFieldMap = {
  temperatureEnabled: 'temperature',
  topPEnabled: 'top_p',
  presencePenaltyEnabled: 'presence_penalty',
  frequencyPenaltyEnabled: 'frequency_penalty',
  maxTokensEnabled: 'max_tokens',
};

export enum SharedFrom {
  Agent = 'agent',
  Chat = 'chat',
}

export enum ChatSearchParams {
  DialogId = 'dialogId',
  ConversationId = 'conversationId',
  isNew = 'isNew',
}

export const EmptyConversationId = 'empty';
