import {
  Dispatch,
  SetStateAction,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { FieldErrors } from 'react-hook-form';

// Elements that flag a validation error: the control marked by FormControl or
// the message paragraph rendered by FormMessage.
const ErrorIndicatorSelector =
  '[aria-invalid="true"], [id$="-form-item-message"]';

// Operator-tab errors surface asynchronously: the tab must mount and mirror
// the outer form's errors onto its fields (a passive effect plus a re-render),
// so the indicator may only appear several frames after the invalid submit.
const MaxPollFrames = 30;

/**
 * Reveals validation errors after an invalid submit: surfaces the operator tab
 * holding the first parser_config error and scrolls the first visible error
 * into view. Only the form's own scroll container is scrolled, never the page.
 * The signal counter re-triggers on every invalid submit, even when the active
 * tab does not change.
 */
export function useRevealSubmitErrors(
  setActiveTab: Dispatch<SetStateAction<string>>,
) {
  const [invalidSubmitSignal, setInvalidSubmitSignal] = useState(0);
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  const handleInvalidSubmit = useCallback(
    (errors: FieldErrors) => {
      // Surface the first failing operator tab so its field errors are visible.
      const firstOperatorId = Object.keys(errors?.parser_config ?? {})[0];
      if (firstOperatorId) {
        setActiveTab(firstOperatorId);
      }
      setInvalidSubmitSignal((signal) => signal + 1);
    },
    [setActiveTab],
  );

  useEffect(() => {
    if (invalidSubmitSignal === 0) {
      return;
    }
    let rafId = 0;
    let polledFrames = 0;
    const scrollToFirstError = () => {
      const container = scrollContainerRef.current;
      const target = container?.querySelector<HTMLElement>(
        ErrorIndicatorSelector,
      );
      if (!container || !target) {
        if (++polledFrames < MaxPollFrames) {
          rafId = requestAnimationFrame(scrollToFirstError);
        }
        return;
      }
      // scrollIntoView({ block: 'center' }) restricted to the form's own
      // scroll container, so the page itself is never pushed around.
      const containerRect = container.getBoundingClientRect();
      const targetRect = target.getBoundingClientRect();
      const top =
        targetRect.top -
        containerRect.top +
        container.scrollTop -
        (container.clientHeight - targetRect.height) / 2;
      container.scrollTo({ top, behavior: 'smooth' });
    };
    rafId = requestAnimationFrame(scrollToFirstError);
    return () => cancelAnimationFrame(rafId);
  }, [invalidSubmitSignal]);

  return { scrollContainerRef, handleInvalidSubmit };
}
