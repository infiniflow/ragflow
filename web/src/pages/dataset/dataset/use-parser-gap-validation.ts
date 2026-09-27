import type { IDocumentInfo } from '@/interfaces/database/document';
import { useIsGoBackend } from '@/utils/backend-variant';
import { useCallback } from 'react';
import { useKnowledgeBaseContext } from '../contexts/knowledge-base-context';
import {
  FileParserGap,
  findDocumentsParserGaps,
  findFilesParserGaps,
  getSavedParserSetups,
} from './utils';

/**
 * Validates that each file can be parsed under the Parser operator config it
 * actually runs with: the file type must be declared in the operator setups,
 * and audio/video additionally need their model configured.
 * Only the Go backend has parser operator settings — on the Python backend
 * every file passes.
 */
export function useParserGapValidation() {
  const isGoBackend = useIsGoBackend();
  const { knowledgeBase, loading } = useKnowledgeBaseContext();

  // Skip while the dataset is loading: its parser_config is not readable
  // yet and every file would look unsupported. A missing Parser entry means
  // the config predates operator-scoped settings, so type support cannot be
  // determined either.
  const findParseGaps = useCallback(
    (names: string[]): FileParserGap[] => {
      if (!isGoBackend || loading) {
        return [];
      }
      const setups = getSavedParserSetups(knowledgeBase);
      if (!setups) {
        return [];
      }
      return findFilesParserGaps(names, setups);
    },
    [isGoBackend, loading, knowledgeBase],
  );

  // Parse-time validation: a document may override the dataset parser, and
  // its row carries the config it actually runs with — validate against that,
  // falling back to the dataset-level config for rows without one.
  const findDocumentParseGaps = useCallback(
    (
      documents: Pick<IDocumentInfo, 'name' | 'parser_config'>[],
    ): FileParserGap[] => {
      if (!isGoBackend || loading) {
        return [];
      }
      return findDocumentsParserGaps(
        documents,
        getSavedParserSetups(knowledgeBase),
      );
    },
    [isGoBackend, loading, knowledgeBase],
  );

  return { findParseGaps, findDocumentParseGaps };
}
