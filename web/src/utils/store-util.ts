export const getOneNamespaceEffectsLoading = (
  namespace: string,
  effects: Record<string, boolean>,
  effectNames: Array<string>,
) => {
  return effectNames.some(
    (effectName) => effects[`${namespace}/${effectName}`],
  );
};

export const delay = (ms: number) =>
  new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
