export function buildSelectOptions(list: Array<string>) {
  return list.map((x) => ({ label: x, value: x }));
}
