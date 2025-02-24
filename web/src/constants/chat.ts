export enum MessageType {
  Assistant = 'assistant',
  User = 'user',
}

export enum ChatVariableEnabledField {
  TemperatureEnabled = 'temperatureEnabled',
  TopPEnabled = 'topPEnabled',
  PresencePenaltyEnabled = 'presencePenaltyEnabled',
  FrequencyPenaltyEnabled = 'frequencyPenaltyEnabled',
  MaxTokensEnabled = 'maxTokensEnabled',
}

export const variableEnabledFieldMap = {
  [ChatVariableEnabledField.TemperatureEnabled]: 'temperature',
  [ChatVariableEnabledField.TopPEnabled]: 'top_p',
  [ChatVariableEnabledField.PresencePenaltyEnabled]: 'presence_penalty',
  [ChatVariableEnabledField.FrequencyPenaltyEnabled]: 'frequency_penalty',
  [ChatVariableEnabledField.MaxTokensEnabled]: 'max_tokens',
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
