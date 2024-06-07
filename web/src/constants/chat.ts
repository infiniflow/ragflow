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
