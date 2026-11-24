type IntegrationItem = { id?: string; name?: string };

export function useVisibleRoleIntegrationItems<T extends IntegrationItem>(
  _feature: 'model_provider' | 'data_source' | 'chat_channel',
  items: T[],
  getItemIDs: (item: T) => string[] = (item) => [item.id ?? item.name ?? ''],
): T[] {
  void getItemIDs;
  return items;
}
