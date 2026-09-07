const imageModules = import.meta.glob<string>(
  '@/assets/chunk-method/*.{png,jpg}',
  { eager: true, query: '?url', import: 'default' },
);

const ImageListByPrefix = Object.entries(imageModules)
  .sort(([a], [b]) => a.localeCompare(b))
  .reduce<Record<string, string[]>>((acc, [path, url]) => {
    const prefix = path.split('/').pop()?.split('-')[0] ?? '';
    (acc[prefix] ??= []).push(url);
    return acc;
  }, {});

// The Go pipeline catalog uses 'general' as the id of the parser that the
// Python backend calls 'naive'; both share the same description.
export const DescriptionKeyMap: Record<string, string> = {
  general: 'naive',
};

export const ImageMap: Record<string, string[]> = {
  book: ImageListByPrefix['book'] ?? [],
  laws: ImageListByPrefix['law'] ?? [],
  manual: ImageListByPrefix['manual'] ?? [],
  picture: ImageListByPrefix['media'] ?? [],
  naive: ImageListByPrefix['naive'] ?? [],
  general: ImageListByPrefix['naive'] ?? [],
  paper: ImageListByPrefix['paper'] ?? [],
  presentation: ImageListByPrefix['presentation'] ?? [],
  qa: ImageListByPrefix['qa'] ?? [],
  resume: ImageListByPrefix['resume'] ?? [],
  table: ImageListByPrefix['table'] ?? [],
  one: ImageListByPrefix['one'] ?? [],
  tag: ImageListByPrefix['tag'] ?? [],
};
