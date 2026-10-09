/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
  Search = 'search',
}

export enum ChatSearchParams {
  DialogId = 'dialogId',
  ConversationId = 'conversationId',
  isNew = 'isNew',
}

export const EmptyConversationId = 'empty';

export enum DatasetMetadata {
  Disabled = 'disabled',
  Automatic = 'auto',
  SemiAutomatic = 'semi_auto',
  Manual = 'manual',
}

export enum WebSearchProvider {
  Brave = 'brave',
  Exa = 'exa',
  Firecrawl = 'firecrawl',
  Linkup = 'linkup',
  Parallel = 'parallel',
  Querit = 'querit',
  Serply = 'serply',
  Tavily = 'tavily',
  YouCom = 'youcom',
}

/**
 * Providers usable with no credentials at all — the key field is optional for
 * these, and the Internet switch stays available when it is left blank.
 *
 * You.com serves a dedicated keyless ENDPOINT: a different path from the keyed
 * one, the same response shape, rate-limited per source IP. Adding a key
 * switches to the keyed endpoint and lifts that limit.
 *
 * "Free tier" is not the same as "keyless". Exa's free tier (1,000
 * requests/month, no credit card) still requires a key on every request, so it
 * is deliberately absent from this list.
 */
export const KEYLESS_WEB_SEARCH_PROVIDERS: readonly WebSearchProvider[] = [
  WebSearchProvider.YouCom,
];
