// A few images in the repo (infiniflow-ai/ragflow-images, builtin/chunk-method)
// are jpg rather than png; override them by 1-based index.
const getImageName = (
  prefix: string,
  length: number,
  extensionOverrides: Record<number, string> = {},
) =>
  new Array(length)
    .fill(0)
    .map(
      (x, idx) =>
        `https://raw.gitcode.com/infiniflow-ai/ragflow-images/raw/main/builtin/chunk-method/${prefix}-0${idx + 1}.${extensionOverrides[idx + 1] ?? 'png'}`,
    );

// The Go pipeline catalog uses 'general' as the id of the parser that the
// Python backend calls 'naive'; both share the same description.
export const DescriptionKeyMap: Record<string, string> = {
  general: 'naive',
};

export const ImageMap = {
  book: getImageName('book', 4, { 1: 'jpg' }),
  laws: getImageName('law', 2),
  manual: getImageName('manual', 4),
  picture: getImageName('media', 2),
  naive: getImageName('naive', 2, { 2: 'jpg' }),
  general: getImageName('naive', 2, { 2: 'jpg' }),
  paper: getImageName('paper', 2),
  presentation: getImageName('presentation', 2),
  qa: getImageName('qa', 2),
  resume: getImageName('resume', 2),
  table: getImageName('table', 2, { 1: 'jpg' }),
  one: getImageName('one', 2),
  // knowledge-graph images are not yet present in the remote repo.
  tag: getImageName('tag', 2),
};
