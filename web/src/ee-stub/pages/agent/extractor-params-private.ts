/**
 * The shared transform calls this in every build; with no extra Extractor
 * sub-tasks here it contributes nothing on top of the shared params.
 */
export const extraExtractorParams = (
  _raw: Record<string, any>,
): Record<string, any> => ({});
