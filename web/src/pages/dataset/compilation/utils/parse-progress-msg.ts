export interface IProgressLogEntry {
  key: number;
  text: string;
  isError: boolean;
}

export const parseProgressMsg = (progressMsg?: string): IProgressLogEntry[] =>
  (progressMsg ?? '')
    .split('\n')
    .filter((line) => line.trim().length > 0)
    .map((line, index) => ({
      key: index,
      text: line,
      isError: line.includes('[ERROR]') || line.startsWith('ERROR:'),
    }));
