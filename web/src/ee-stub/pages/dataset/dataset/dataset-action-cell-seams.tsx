import { IDocumentInfo } from '@/interfaces/database/document';

export type DocumentActionCellOverrides = {
  actionClassName?: string;
};

export type DocumentPrivilegeActionProps = {
  record: IDocumentInfo;
  disabled: boolean;
};

const overrides: DocumentActionCellOverrides = {};

/**
 * Enterprise seams for the document row actions. Open source keeps the upstream
 * hover-only controls and exposes no permission entry.
 */
export function useDocumentActionCellOverrides(): DocumentActionCellOverrides {
  return overrides;
}

export function DocumentPrivilegeAction(_props: DocumentPrivilegeActionProps) {
  void _props;
  return null;
}
