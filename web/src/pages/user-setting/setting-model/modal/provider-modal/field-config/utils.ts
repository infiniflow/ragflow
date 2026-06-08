/**
 * Capitalize the first letter of a string
 */
export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/**
 * When model_type contains chat and vision=true, automatically add image2text
 */
export function applyChatToImage2Text(
  modelType: string[] | string | undefined,
  vision?: boolean,
): string[] {
  const arr = Array.isArray(modelType)
    ? modelType
    : modelType
      ? [modelType]
      : [];
  if (arr.includes('chat') && vision) {
    return [...arr, 'image2text'];
  }
  return arr;
}
