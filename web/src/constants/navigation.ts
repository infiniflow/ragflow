export const NavigationSections = [
  'home',
  'dataset',
  'chat',
  'search',
  'agent',
  'memory',
  'catalog',
  'business_documents',
  'file_manager',
] as const;

export type NavigationSection = (typeof NavigationSections)[number];

export const isNavigationSection = (
  value: unknown,
): value is NavigationSection =>
  typeof value === 'string' &&
  NavigationSections.includes(value as NavigationSection);
