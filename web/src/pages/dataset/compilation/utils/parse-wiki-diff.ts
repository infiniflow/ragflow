import type {
  WikiDiffHunk,
  WikiDiffLine,
  WikiDiffLineType,
} from '../interface';

export function parseWikiDiff(diff: string): WikiDiffHunk[] {
  const rawLines = (diff || '').split('\n');
  const hunks: WikiDiffHunk[] = [];
  let currentHunk: WikiDiffHunk | null = null;

  for (const rawLine of rawLines) {
    if (rawLine.startsWith('@@')) {
      if (currentHunk) {
        hunks.push(currentHunk);
      }
      currentHunk = { header: rawLine, lines: [] };
      continue;
    }

    if (rawLine.startsWith('---') || rawLine.startsWith('+++')) {
      // File header — not rendered per-line.
      continue;
    }

    let type: WikiDiffLineType = 'context';
    let content = rawLine;

    if (rawLine.startsWith('+')) {
      type = 'added';
      content = rawLine.slice(1);
    } else if (rawLine.startsWith('-')) {
      type = 'removed';
      content = rawLine.slice(1);
    }

    if (currentHunk) {
      currentHunk.lines.push({ type, content });
    }
  }

  if (currentHunk) {
    hunks.push(currentHunk);
  }

  return hunks;
}

export function getWikiDiffChanges(hunks: WikiDiffHunk[]): WikiDiffLine[] {
  return hunks
    .flatMap((hunk) => hunk.lines)
    .filter((line) => line.type === 'added' || line.type === 'removed');
}
