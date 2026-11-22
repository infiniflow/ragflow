import type { IHighlight } from 'react-pdf-highlighter';

export function supportsAdditionalSourceLocate(_extension?: string) {
  void _extension;
  return false;
}

export function getSourceLocatePageNumber(_highlights?: IHighlight[]) {
  void _highlights;
  return 1;
}
