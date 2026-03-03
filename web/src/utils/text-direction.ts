/**
 * RTL (Right-to-Left) text direction utilities
 * Supports Arabic, Hebrew, Persian/Farsi, Urdu, and other RTL scripts
 */

// Unicode ranges for RTL scripts
const RTL_RANGES: [number, number][] = [
  [0x0600, 0x06ff], // Arabic
  [0x0750, 0x077f], // Arabic Supplement
  [0x08a0, 0x08ff], // Arabic Extended-A
  [0xfb50, 0xfdff], // Arabic Presentation Forms-A
  [0xfe70, 0xfeff], // Arabic Presentation Forms-B
  [0x0590, 0x05ff], // Hebrew
  [0xfb1d, 0xfb4f], // Hebrew Presentation Forms
  [0x0700, 0x074f], // Syriac
  [0x0780, 0x07bf], // Thaana (Maldivian)
  [0x0840, 0x085f], // Mandaic
  [0x0860, 0x086f], // Syriac Supplement
];

/**
 * Check if a character code is in RTL Unicode range
 */
const isRTLCharCode = (charCode: number): boolean => {
  return RTL_RANGES.some(
    ([start, end]) => charCode >= start && charCode <= end,
  );
};

/**
 * Find the first "strong" directional character in text
 * Strong characters are letters (not numbers, punctuation, or whitespace)
 * Returns 'rtl', 'ltr', or 'neutral' if no strong character found
 */
export const getTextDirection = (text: string): 'rtl' | 'ltr' | 'neutral' => {
  if (!text) return 'neutral';

  for (const char of text) {
    const code = char.charCodeAt(0);

    // Skip whitespace, numbers, and common punctuation
    if (
      code <= 0x40 || // Control chars, digits, basic punctuation
      (code >= 0x5b && code <= 0x60) || // [ \ ] ^ _ `
      (code >= 0x7b && code <= 0x7f) // { | } ~ DEL
    ) {
      continue;
    }

    // Check if RTL
    if (isRTLCharCode(code)) {
      return 'rtl';
    }

    // If we found a non-RTL letter, it's LTR
    // Latin, Greek, Cyrillic, etc.
    if (
      (code >= 0x41 && code <= 0x5a) || // A-Z
      (code >= 0x61 && code <= 0x7a) || // a-z
      (code >= 0x00c0 && code <= 0x024f) || // Latin Extended
      (code >= 0x0370 && code <= 0x03ff) || // Greek
      (code >= 0x0400 && code <= 0x04ff) // Cyrillic
    ) {
      return 'ltr';
    }
  }

  return 'neutral';
};

/**
 * Check if text contains any RTL characters
 * Useful for detecting mixed content
 */
export const containsRTL = (text: string): boolean => {
  if (!text) return false;

  for (const char of text) {
    if (isRTLCharCode(char.charCodeAt(0))) {
      return true;
    }
  }
  return false;
};

/**
 * Check if text is predominantly RTL
 * Returns true if first strong character is RTL
 */
export const isRTL = (text: string): boolean => {
  return getTextDirection(text) === 'rtl';
};

/**
 * Get the appropriate dir attribute value for HTML elements
 * Returns 'rtl', 'ltr', or 'auto' (for neutral/mixed content)
 */
export const getDirAttribute = (text: string): 'rtl' | 'ltr' | 'auto' => {
  const direction = getTextDirection(text);
  return direction === 'neutral' ? 'auto' : direction;
};
