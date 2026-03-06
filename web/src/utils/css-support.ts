export const supportsCssAnchor =
  CSS.supports('position-anchor', '--anchor-name') &&
  CSS.supports('anchor-name', '--anchor-name') &&
  CSS.supports('top', 'anchor(--anchor-name bottom)') &&
  CSS.supports('width', 'anchor-size(--anchor-name width)');
