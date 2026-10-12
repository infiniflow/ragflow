import { useCallback } from 'react';
import type { UseFormReturn, UseFormSetValue } from 'react-hook-form';
import { ParseType } from '@/constants/knowledge';
import { ParserOptionKind } from './use-parser-options';

// The three parser forms (dataset settings, dataset creation, per-document
// pipeline) all carry this subset of fields. Constraining the handler to it
// guarantees at the call site that the form actually has these fields.
interface ParserFormFields {
  parse_type?: ParseType;
  parser_id?: string;
  pipeline_id?: string;
}

// useParserSelectHandler returns a stable onChange handler for ParserSelect that
// maps the resolved (kind, rawId) pair onto the form's parser fields. Extracting
// it keeps the duplicated logic in one place and avoids a fresh closure on every
// render, which would otherwise defeat SelectWithSearch's memoization.
export function useParserSelectHandler<T extends ParserFormFields>(
  form: UseFormReturn<T>,
) {
  return useCallback(
    (kind: ParserOptionKind | null, rawId: string) => {
      // UseFormReturn is invariant in its values type, so the full form cannot
      // be narrowed to ParserFormFields directly. The constraint above
      // guarantees these fields exist; narrowing setValue once keeps the field
      // names and value types below checked.
      const setValue =
        form.setValue as unknown as UseFormSetValue<ParserFormFields>;
      if (kind === ParserOptionKind.BuiltIn) {
        setValue('parse_type', ParseType.BuiltIn);
        setValue('parser_id', rawId);
        setValue('pipeline_id', '');
      } else if (kind === ParserOptionKind.Pipeline) {
        setValue('parse_type', ParseType.Pipeline);
        setValue('pipeline_id', rawId);
        setValue('parser_id', '');
      } else {
        setValue('parser_id', '');
        setValue('pipeline_id', '');
      }
    },
    [form],
  );
}
