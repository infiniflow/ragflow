import { IModalProps } from '@/interfaces/common';

/**
 * Enterprise dialog for managing a resource's collaborators. Open source has
 * no collaborator permissions, so the slot renders nothing; the shared call
 * sites still mount it with the same props. The prop type stays opaque here
 * because the collaborator record only has a shape in the enterprise build.
 */
export function PrivilegeManagementDialog(
  _props: IModalProps<any> & {
    initialValues: Record<string, unknown>;
  },
) {
  return null;
}
