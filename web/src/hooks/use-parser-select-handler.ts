import { useCallback } from 'react';
import type { UseFormReturn } from 'react-hook-form';
import { ParseType } from '@/constants/knowledge';
import type { ParserOptionKind } from './use-parser-options';

// useParserSelectHandler returns a stable onChange handler for ParserSelect that
// maps the resolved (kind, rawId) pair onto the form's parser fields. Extracting
// it keeps the duplicated logic in one place and avoids a fresh closure on every
// render, which would otherwise defeat SelectWithSearch's memoization.
export function useParserSelectHandler(form: UseFormReturn<any>) {
  return useCallback(
    (kind: ParserOptionKind | null, rawId: string) => {
      if (kind === 'builtin') {
        form.setValue('parse_type', ParseType.BuiltIn);
        form.setValue('parser_id', rawId);
        form.setValue('pipeline_id', '');
      } else if (kind === 'pipeline') {
        form.setValue('parse_type', ParseType.Pipeline);
        form.setValue('pipeline_id', rawId);
        form.setValue('parser_id', '');
      } else {
        form.setValue('parser_id', '');
        form.setValue('pipeline_id', '');
      }
    },
    [form],
  );
}
