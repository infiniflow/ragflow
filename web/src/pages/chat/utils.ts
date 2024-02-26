import { EmptyConversationId, variableEnabledFieldMap } from './constants';

export const excludeUnEnabledVariables = (values: any) => {
  const unEnabledFields: Array<keyof typeof variableEnabledFieldMap> =
    Object.keys(variableEnabledFieldMap).filter((key) => !values[key]) as Array<
      keyof typeof variableEnabledFieldMap
    >;

  return unEnabledFields.map(
    (key) => `llm_setting.${variableEnabledFieldMap[key]}`,
  );
};

export const isConversationIdNotExist = (conversationId: string) => {
  return conversationId !== EmptyConversationId && conversationId !== '';
};
