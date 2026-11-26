import type { IDocumentInfo } from '@/interfaces/database/document';
import type { ReactNode } from 'react';

export function ParsingErrorLabel({
  children,
}: {
  record: IDocumentInfo;
  children: ReactNode;
}) {
  return children;
}
