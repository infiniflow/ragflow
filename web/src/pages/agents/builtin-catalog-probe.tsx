import { useEffect } from 'react';
import { useBuiltinItems } from './builtin-pipeline-list';

// Reports the filtered built-in pipeline catalog state (matched item count and
// loading flag) up to the agents list so it can decide the empty-state without
// re-deriving the filtering logic. Mounted only when the built-in section is
// category-eligible, so the catalog request stays gated on the "All"/"Pipeline"
// views (React Query dedupes it with the section's own call).
export function BuiltinCatalogProbe({
  rawCategory,
  searchString,
  onReport,
}: {
  rawCategory?: string | string[] | Record<string, string[]>;
  searchString?: string;
  onReport: (state: { length: number; loading: boolean }) => void;
}) {
  const { items, loading } = useBuiltinItems(rawCategory, searchString);

  useEffect(() => {
    onReport({ length: items.length, loading });
  }, [items.length, loading, onReport]);

  return null;
}
