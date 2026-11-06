import { IModalProps } from '@/interfaces/common';

/**
 * Enterprise dialog for duplicating a resource. Open source has no duplicate
 * action, so the slot renders nothing; the shared call sites still mount it
 * with the same props.
 */
export default function DuplicateDialog(
  _props: IModalProps<any> & { initialName?: string },
) {
  return null;
}
