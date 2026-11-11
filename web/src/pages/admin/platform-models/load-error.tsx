import { useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';

import { LucideTriangleAlert } from 'lucide-react';

import { Button } from '@/components/ui/button';

/** Shown when platform data cannot be loaded, so it is not taken as empty. */
export function LoadError({
  onRetry,
  message,
}: {
  onRetry: () => unknown;
  /** What failed to load; the platform models by default. */
  message?: string;
}) {
  const { t } = useTranslation();
  const alertRef = useRef<HTMLDivElement>(null);
  const markerRef = useRef<Element | null>(null);
  // A successful retry replaces this alert; focus then moves to the first
  // control of the content that replaced it instead of falling to the page.
  const handleRetry = useCallback(async () => {
    const container = alertRef.current?.parentElement ?? null;
    // Remember the alert's place by the element before it, which stays when
    // the alert is replaced.
    markerRef.current = alertRef.current?.previousElementSibling ?? null;
    await onRetry();
    window.setTimeout(() => {
      // Focus is lost when it fell to the page or to the dialog box itself.
      const active = document.activeElement;
      const lost =
        !active ||
        active === document.body ||
        !active.isConnected ||
        active.getAttribute('role') === 'dialog';
      if (!lost) return;
      // The first control after where the alert stood, i.e. in the content
      // that replaced it; controls before it (a heading, "Add") are not new.
      const marker = markerRef.current;
      const controls = Array.from(
        container?.querySelectorAll<HTMLElement>(
          'button:not([disabled]), [href], input:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      );
      const after = marker?.isConnected
        ? controls.find((control) => {
            const position = marker.compareDocumentPosition(control);
            return (
              (position & Node.DOCUMENT_POSITION_FOLLOWING) !== 0 &&
              (position & Node.DOCUMENT_POSITION_CONTAINED_BY) === 0
            );
          })
        : undefined;
      (after ?? controls[0])?.focus();
    }, 50);
  }, [onRetry]);
  return (
    <div
      ref={alertRef}
      role="alert"
      className="flex flex-wrap items-center gap-3 px-6 py-4 text-sm text-state-error"
    >
      <LucideTriangleAlert className="size-4 shrink-0" />
      <span className="min-w-0 break-words">
        {message ?? t('admin.platformModelsPage.loadFailed')}
      </span>
      <Button
        variant="outline"
        className="h-8 dark:border-border-button"
        onClick={handleRetry}
      >
        {t('admin.platformModelsPage.retry')}
      </Button>
    </div>
  );
}
