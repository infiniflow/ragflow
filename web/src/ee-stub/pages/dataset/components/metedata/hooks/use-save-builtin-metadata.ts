import {
  IBuiltInMetadataItem,
  IMetaDataReturnJSONSettings,
} from '@/pages/dataset/components/metedata/interface';
import { useCallback } from 'react';

export type SaveBuiltinMetadataParams = {
  kbId?: string;
  documentId?: string;
  builtInMetadata?: IBuiltInMetadataItem[];
  metadata: IMetaDataReturnJSONSettings;
};

/**
 * Enterprise seam for persisting built-in metadata alongside a single
 * document update. Open source has no collaborator permissions, so the shared
 * save flow already covers it.
 */
export function useSaveBuiltinMetadata() {
  const saveBuiltinMetadata = useCallback(
    async (_params: SaveBuiltinMetadataParams) => {
      void _params;
    },
    [],
  );

  return { saveBuiltinMetadata };
}
