type NestedObject = {
  [key: string]: string | NestedObject;
};

type FlattenedObject = {
  [key: string]: string;
};

export function flattenObject(
  obj: NestedObject,
  parentKey: string = '',
): FlattenedObject {
  const result: FlattenedObject = {};

  for (const [key, value] of Object.entries(obj)) {
    const newKey = parentKey ? `${parentKey}.${key}` : key;

    if (typeof value === 'object' && value !== null) {
      Object.assign(result, flattenObject(value as NestedObject, newKey));
    } else {
      result[newKey] = value as string;
    }
  }

  return result;
}
type TranslationTableRow = {
  key: string;
  [language: string]: string;
};

/**
 * Creates a translation table from multiple flattened language objects.
 * @param langs - A list of flattened language objects.
 * @param langKeys - A list of language identifiers (e.g., 'English', 'Vietnamese').
 * @returns An array representing the translation table.
 */
export function createTranslationTable(
  langs: FlattenedObject[],
  langKeys: string[],
): TranslationTableRow[] {
  const keys = new Set<string>();

  // Collect all unique keys from the language objects
  langs.forEach((lang) => {
    Object.keys(lang).forEach((key) => keys.add(key));
  });

  // Build the table
  return Array.from(keys).map((key) => {
    const row: TranslationTableRow = { key };

    langs.forEach((lang, index) => {
      const langKey = langKeys[index];
      row[langKey] = lang[key] || ''; // Use empty string if key is missing
    });

    return row;
  });
}
