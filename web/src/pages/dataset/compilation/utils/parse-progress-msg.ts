export interface IProgressLogEntry {
  key: number;
  text: string;
  isError: boolean;
  isWarning: boolean;
  isRunning: boolean;
  isInfo: boolean;
}

export const parseProgressMsg = (progressMsg?: string): IProgressLogEntry[] =>
  (progressMsg ?? '')
    .split('\n')
    .filter((line) => line.trim().length > 0)
    .map((line, index) => ({
      key: index,
      text: line,
      isError: line.includes('[ERROR]') || line.startsWith('ERROR:'),
      isWarning: line.includes('[WARN]'),
      isRunning: line.includes('[RUNNING]'),
      isInfo: line.includes('[INFO]'),
    }));
