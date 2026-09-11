import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useCallback } from 'react';
import { useParams } from 'react-router';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import {
  FileModelGap,
  findFilesMissingParserModels,
  getEffectiveParserSetups,
} from './utils';

/**
 * Validates that the models required to parse a file type (audio/video/image)
 * are configured on the dataset's Parser operator.
 * Only the Go backend has parser operator settings — on the Python backend
 * every file passes.
 */
export function useParserModelValidation() {
  const isGoBackend = useIsGoBackend();
  const { knowledgeBase, loading } = useKnowledgeBaseContext();
  const { id } = useParams();
  const { navigateToDatasetSetting } = useNavigatePage();

  const findFilesMissingModels = useCallback(
    (names: string[]): FileModelGap[] => {
      // Skip while the dataset is loading: its parser_config is not readable
      // yet and every media file would look unconfigured.
      if (!isGoBackend || loading) {
        return [];
      }
      return findFilesMissingParserModels(
        names,
        getEffectiveParserSetups(knowledgeBase),
      );
    },
    [isGoBackend, loading, knowledgeBase],
  );

  const goToDatasetConfiguration = useCallback(() => {
    if (id) {
      navigateToDatasetSetting(id);
    }
  }, [id, navigateToDatasetSetting]);

  return { findFilesMissingModels, goToDatasetConfiguration };
}
