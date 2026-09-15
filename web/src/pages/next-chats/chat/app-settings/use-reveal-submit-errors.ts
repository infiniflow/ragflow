import { useCallback, useLayoutEffect, useRef, useState } from 'react';

// Elements that flag a validation error: the control marked by FormControl or
// the message paragraph rendered by FormMessage.
const ErrorIndicatorSelector =
  '[aria-invalid="true"], [id$="-form-item-message"]';

/**
 * Reveals validation errors after an invalid submit: expands every collapsed
 * settings section and scrolls the first error into view. The signal counter
 * re-triggers the layout effect on every invalid submit, even when the
 * sections are already open.
 */
export function useRevealSubmitErrors() {
  const [modelSettingOpen, setModelSettingOpen] = useState(false);
  const [advancedSettingOpen, setAdvancedSettingOpen] = useState(false);
  const [invalidSubmitSignal, setInvalidSubmitSignal] = useState(0);
  const formContainerRef = useRef<HTMLFormElement>(null);

  const handleInvalidSubmit = useCallback(() => {
    setModelSettingOpen(true);
    setAdvancedSettingOpen(true);
    setInvalidSubmitSignal((signal) => signal + 1);
  }, []);

  useLayoutEffect(() => {
    if (invalidSubmitSignal === 0) {
      return;
    }
    const firstErrorIndicator =
      formContainerRef.current?.querySelector<HTMLElement>(
        ErrorIndicatorSelector,
      );
    firstErrorIndicator?.scrollIntoView({
      behavior: 'smooth',
      block: 'center',
    });
  }, [invalidSubmitSignal]);

  return {
    formContainerRef,
    handleInvalidSubmit,
    modelSettingOpen,
    onModelSettingOpenChange: setModelSettingOpen,
    advancedSettingOpen,
    onAdvancedSettingOpenChange: setAdvancedSettingOpen,
  };
}
