export const getOneNamespaceEffectsLoading = (
  namespace: string,
  effects: Record<string, boolean>,
  effectNames: Array<string>,
) => {
  return effectNames.some(
    (effectName) => effects[`${namespace}/${effectName}`],
  );
};
