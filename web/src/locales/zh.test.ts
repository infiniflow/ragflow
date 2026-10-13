import { expect, it } from '@jest/globals';
import en from './en';
import zh from './zh';

function flatten(value: object, prefix = ''): Record<string, string> {
  return Object.fromEntries(
    Object.entries(value).flatMap(([key, entry]) => {
      const name = prefix ? `${prefix}.${key}` : key;
      return typeof entry === 'object'
        ? Object.entries(flatten(entry, name))
        : [[name, entry]];
    }),
  );
}

it('provides Chinese translations for every English key with matching interpolation', () => {
  const english = flatten(en.translation);
  const chinese = flatten(zh.translation);
  const placeholders = (value: string) =>
    [...value.matchAll(/\{\{[^}]+\}\}/g)].map((match) => match[0]).sort();
  for (const [key, value] of Object.entries(english)) {
    expect(chinese[key]).toBeDefined();
    expect(placeholders(chinese[key])).toEqual(placeholders(value));
  }
});
