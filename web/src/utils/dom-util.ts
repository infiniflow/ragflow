import DOMPurify from 'dompurify';

export const scrollToBottom = (element: HTMLElement) => {
  element.scrollTo(0, element.scrollHeight);
};

/**
 * Sanitize HTML and render any <img> elements as escaped text strings
 * instead of loading them as images. Prevents image-based tracking /
 * data exfiltration via external image URLs and layout disruption, while
 * keeping the img markup visible to the user as literal text.
 *
 * Event-handler XSS (onerror) is already stripped by DOMPurify's default
 * config; this additionally neutralizes the image element itself.
 */
export const sanitizeHtmlWithImagesAsText = (html: string): string => {
  const node = DOMPurify.sanitize(html, { RETURN_DOM: true }) as HTMLElement;
  node.querySelectorAll('img').forEach((img) => {
    img.replaceWith(document.createTextNode(img.outerHTML));
  });
  return node.innerHTML;
};
