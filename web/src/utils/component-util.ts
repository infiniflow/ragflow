export function buildSelectOptions(
  list: Array<any>,
  keyName?: string,
  valueName?: string,
) {
  if (!Array.isArray(list) || !list.length) {
    return [];
  }
  if (keyName && valueName) {
    return list.map((x) => ({ label: x[valueName], value: x[keyName] }));
  }
  return list.map((x) => ({ label: x, value: x }));
}
