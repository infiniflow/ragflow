import { IFile } from '@/interfaces/database/file-manager';

type FilePrivilegeButtonProps = {
  record: IFile;
  showPrivilegeModal?(record: IFile): void;
};

/**
 * Enterprise seam for the file permission entry point. Open source has no
 * collaborator permissions, so the control is never rendered.
 */
export function FilePrivilegeButton(_props: FilePrivilegeButtonProps) {
  void _props;
  return null;
}
