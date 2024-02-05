import { getOneNamespaceEffectsLoading } from '@/utils/storeUtil';
import { useSelector } from 'umi';

// Get the loading status of given effects under a certain namespace
export const useOneNamespaceEffectsLoading = (
  namespace: string,
  effectNames: Array<string>,
) => {
  const effects = useSelector((state: any) => state.loading.effects);
  return getOneNamespaceEffectsLoading(namespace, effects, effectNames);
};
