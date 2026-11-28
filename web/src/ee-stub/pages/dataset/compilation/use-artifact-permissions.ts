import type { IArtifactPage } from '@/interfaces/database/dataset';

export function useArtifactPermissions(
  _page: IArtifactPage | null | undefined,
  isVersionView: boolean,
) {
  return { canEdit: !isVersionView };
}
