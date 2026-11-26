import type { Control } from 'react-hook-form';

/**
 * The shared extractor form mounts this slot in every build; without an
 * extra extraction mode registered here there is nothing to render and no
 * group is hidden.
 */
export const ExtractorResumeField = () => null;

export const useExtractorResumeExclusive = (_control: Control<any>): boolean =>
  false;
