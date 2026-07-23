import llmService from '@/services/llm-service';
import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * Drives the AIMLAPI agent-authorization (OAuth 2.0 device grant) flow from the
 * provider dialog: start → open the AIMLAPI consent page in a popup → poll the
 * RAGFlow backend until the key is issued. RAGFlow never sees the user's AIMLAPI
 * credentials; the popup handles sign-in/consent on aimlapi.com.
 *
 * Mirrors the data-source connector OAuth pattern
 * (`data-source/component/google-drive-token-field.tsx`): a popup opened
 * synchronously on click, a self-scheduling `setTimeout` poll loop, a manual
 * "check status" fallback, and cleanup on unmount.
 */
export type AimlapiAuthorizeStatus =
  | 'idle'
  | 'starting'
  | 'awaiting_consent'
  | 'success'
  | 'denied'
  | 'expired'
  | 'error';

const TERMINAL_DENIED = new Set(['denied', 'rejected']);
const TERMINAL_EXPIRED = new Set(['expired', 'cancelled', 'canceled']);
const POPUP_NAME = 'ragflow-aimlapi-oauth';
const POPUP_FEATURES = 'width=600,height=760';

interface UseAimlapiAuthorizeOptions {
  onKey: (apiKey: string) => void;
}

export function useAimlapiAuthorize({ onKey }: UseAimlapiAuthorizeOptions) {
  const [status, setStatus] = useState<AimlapiAuthorizeStatus>('idle');
  const [error, setError] = useState<string | null>(null);

  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const deadlineRef = useRef<number>(0);
  const requestIdRef = useRef<string | null>(null);
  const popupRef = useRef<Window | null>(null);
  const stoppedRef = useRef<boolean>(false);
  const intervalMsRef = useRef<number>(5000);

  const clearTimer = useCallback(() => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    return () => {
      stoppedRef.current = true;
      clearTimer();
    };
  }, [clearTimer]);

  const poll = useCallback(
    async (requestId: string, intervalMs: number) => {
      if (stoppedRef.current) return;
      if (Date.now() > deadlineRef.current) {
        setStatus('expired');
        return;
      }
      try {
        const { data } = await llmService.aimlapiAuthorizePoll({
          request_id: requestId,
        });
        if (stoppedRef.current) return;
        if (data?.code !== 0) {
          setStatus('error');
          setError(data?.message ?? null);
          return;
        }
        const result = data.data ?? {};
        const state: string = result.status ?? 'pending';
        if (state === 'ready' && result.api_key) {
          setStatus('success');
          try {
            popupRef.current?.close();
          } catch {
            /* popup may be cross-origin/closed */
          }
          onKey(result.api_key);
          return;
        }
        if (TERMINAL_DENIED.has(state)) {
          setStatus('denied');
          return;
        }
        if (TERMINAL_EXPIRED.has(state)) {
          setStatus('expired');
          return;
        }
        // still pending → schedule the next poll
        pollTimerRef.current = setTimeout(
          () => poll(requestId, intervalMs),
          intervalMs,
        );
      } catch (e: any) {
        if (stoppedRef.current) return;
        setStatus('error');
        setError(e?.message ?? null);
      }
    },
    [onKey],
  );

  const start = useCallback(async () => {
    stoppedRef.current = false;
    setError(null);
    setStatus('starting');
    clearTimer();

    // Open the popup synchronously within the click handler so it is not blocked;
    // navigate it once the backend returns the consent URL.
    const popup = window.open('', POPUP_NAME, POPUP_FEATURES);
    popupRef.current = popup;

    try {
      const { data } = await llmService.aimlapiAuthorizeStart({
        return_url: window.location.origin,
      });
      if (data?.code !== 0) {
        setStatus('error');
        setError(data?.message ?? null);
        try {
          popup?.close();
        } catch {
          /* noop */
        }
        return;
      }
      const {
        request_id: requestId,
        verification_uri: verificationUri,
        interval,
        expires_in: expiresIn,
      } = data.data ?? {};

      requestIdRef.current = requestId;
      deadlineRef.current = Date.now() + (expiresIn ?? 900) * 1000;

      if (popup && verificationUri) {
        popup.location.href = verificationUri;
      } else if (verificationUri) {
        window.open(verificationUri, POPUP_NAME, POPUP_FEATURES);
      }

      setStatus('awaiting_consent');
      const intervalMs = Math.max(interval ?? 5, 1) * 1000;
      intervalMsRef.current = intervalMs;
      pollTimerRef.current = setTimeout(
        () => poll(requestId, intervalMs),
        intervalMs,
      );
    } catch (e: any) {
      setStatus('error');
      setError(e?.message ?? null);
      try {
        popup?.close();
      } catch {
        /* noop */
      }
    }
  }, [clearTimer, poll]);

  // Manual "check now" fallback (e.g. if the popup's postMessage/redirect is missed).
  const checkNow = useCallback(() => {
    const requestId = requestIdRef.current;
    if (!requestId || stoppedRef.current) return;
    clearTimer();
    poll(requestId, intervalMsRef.current);
  }, [clearTimer, poll]);

  return { status, error, start, checkNow };
}
