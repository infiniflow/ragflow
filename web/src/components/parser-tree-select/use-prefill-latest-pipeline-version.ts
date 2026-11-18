import { useEffect } from 'react';
import type { UseFormReturn, UseFormSetValue } from 'react-hook-form';

interface PipelineVersionField {
  pipeline_version_id?: string;
}

// Design section 8 (T37): an unpinned dataset/document must not keep an empty
// version — once the selected pipeline's version list resolves, prefill the
// field with the latest concrete version id so the merged parser tree select
// displays an actual version leaf. Only an empty field is filled, so a saved
// pin or a manual selection always wins. `latestVersionId` is reactive (it
// resolves after the async fetch), so this re-fires when the list arrives and
// stays silent while the id is stable.
export function usePrefillLatestPipelineVersion<T extends PipelineVersionField>(
  form: UseFormReturn<T>,
  savedPinnedVersionId: string | undefined,
  latestVersionId: string | undefined,
) {
  useEffect(() => {
    // Same narrowing trick as useParserSelectHandler: UseFormReturn is
    // invariant in its values type, and the constraint above guarantees the
    // field exists.
    const narrowed = form as unknown as UseFormReturn<PipelineVersionField>;
    if (
      !savedPinnedVersionId &&
      latestVersionId &&
      !narrowed.getValues('pipeline_version_id')
    ) {
      const setValue =
        form.setValue as unknown as UseFormSetValue<PipelineVersionField>;
      setValue('pipeline_version_id', latestVersionId);
    }
  }, [form, savedPinnedVersionId, latestVersionId]);
}
