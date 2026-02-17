import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import message from '@/components/ui/message';
import {
  pollBoxWebAuthResult,
  startBoxWebAuth,
} from '@/services/data-source-service';
import { Loader2 } from 'lucide-react';

export type BoxTokenFieldProps = {
  value?: string;
  onChange: (value: any) => void;
  placeholder?: string;
};

type BoxCredentials = {
  client_id?: string;
  client_secret?: string;
  redirect_uri?: string;
  authorization_code?: string;
  access_token?: string;
  refresh_token?: string;
};

type BoxAuthStatus = 'idle' | 'waiting' | 'success' | 'error';

const parseBoxCredentials = (content?: string): BoxCredentials | null => {
  if (!content) return null;
  try {
    const parsed = JSON.parse(content);
    return {
      client_id: parsed.client_id,
      client_secret: parsed.client_secret,
      redirect_uri: parsed.redirect_uri,
      authorization_code: parsed.authorization_code ?? parsed.code,
      access_token: parsed.access_token,
      refresh_token: parsed.refresh_token,
    };
  } catch {
    return null;
  }
};

const BoxTokenField = ({ value, onChange }: BoxTokenFieldProps) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [redirectUri, setRedirectUri] = useState('');
  const [submitLoading, setSubmitLoading] = useState(false);
  const [webFlowId, setWebFlowId] = useState<string | null>(null);
  const webFlowIdRef = useRef<string | null>(null);
  const webPollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [webStatus, setWebStatus] = useState<BoxAuthStatus>('idle');
  const [webStatusMessage, setWebStatusMessage] = useState('');

  const parsed = useMemo(() => parseBoxCredentials(value), [value]);
  const parsedRedirectUri = useMemo(() => parsed?.redirect_uri ?? '', [parsed]);

  useEffect(() => {
    if (!dialogOpen) {
      setClientId(parsed?.client_id ?? '');
      setClientSecret(parsed?.client_secret ?? '');
      setRedirectUri(parsed?.redirect_uri ?? '');
    }
  }, [parsed, dialogOpen]);

  useEffect(() => {
    webFlowIdRef.current = webFlowId;
  }, [webFlowId]);

  useEffect(() => {
    return () => {
      if (webPollTimerRef.current) {
        clearTimeout(webPollTimerRef.current);
      }
    };
  }, []);

  const hasConfigured = useMemo(
    () =>
      Boolean(
        parsed?.client_id && parsed?.client_secret && parsed?.redirect_uri,
      ),
    [parsed],
  );

  const hasAuthorized = useMemo(
    () =>
      Boolean(
        parsed?.access_token ||
        parsed?.refresh_token ||
        parsed?.authorization_code,
      ),
    [parsed],
  );

  const resetWebStatus = useCallback(() => {
    setWebStatus('idle');
    setWebStatusMessage('');
  }, []);

  const clearWebState = useCallback(() => {
    if (webPollTimerRef.current) {
      clearTimeout(webPollTimerRef.current);
      webPollTimerRef.current = null;
    }
    webFlowIdRef.current = null;
    setWebFlowId(null);
  }, []);

  const fetchWebResult = useCallback(
    async (flowId: string) => {
      try {
        const { data } = await pollBoxWebAuthResult({ flow_id: flowId });
        if (data.code === 0 && data.data?.credentials) {
          const credentials = (data.data.credentials || {}) as Record<
            string,
            any
          >;
          const { user_id: _userId, code, ...rest } = credentials;

          const finalValue: Record<string, any> = {
            ...rest,
            client_id: rest.client_id ?? clientId.trim(),
            client_secret: rest.client_secret ?? clientSecret.trim(),
          };

          const redirect =
            redirectUri.trim() || parsedRedirectUri || rest.redirect_uri;
          if (redirect) {
            finalValue.redirect_uri = redirect;
          }

          if (code) {
            finalValue.authorization_code = code;
          }

          onChange(JSON.stringify(finalValue));
          message.success('Box authorization completed.');
          clearWebState();
          resetWebStatus();
          setDialogOpen(false);
          return;
        }

        if (data.code === 106) {
          setWebStatus('waiting');
          setWebStatusMessage(
            'Authorization confirmed. Finalizing credentials...',
          );
          if (webPollTimerRef.current) {
            clearTimeout(webPollTimerRef.current);
          }
          webPollTimerRef.current = setTimeout(
            () => fetchWebResult(flowId),
            1500,
          );
          return;
        }

        const errorMessage = data.message || 'Authorization failed.';
        message.error(errorMessage);
        setWebStatus('error');
        setWebStatusMessage(errorMessage);
        clearWebState();
      } catch (_error) {
        message.error('Unable to retrieve authorization result.');
        setWebStatus('error');
        setWebStatusMessage('Unable to retrieve authorization result.');
        clearWebState();
      }
    },
    [
      clearWebState,
      clientId,
      clientSecret,
      parsedRedirectUri,
      redirectUri,
      resetWebStatus,
      onChange,
    ],
  );

  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const payload = event.data;
      if (!payload || payload.type !== 'ragflow-box-oauth') {
        return;
      }

      const targetFlowId = payload.flowId || webFlowIdRef.current;
      if (!targetFlowId) return;
      if (webFlowIdRef.current && webFlowIdRef.current !== targetFlowId) {
        return;
      }

      if (payload.status === 'success') {
        setWebStatus('waiting');
        setWebStatusMessage(
          'Authorization confirmed. Finalizing credentials...',
        );
        fetchWebResult(targetFlowId);
      } else {
        const errorMessage = payload.message || 'Authorization failed.';
        message.error(errorMessage);
        setWebStatus('error');
        setWebStatusMessage(errorMessage);
        clearWebState();
      }
    };

    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [clearWebState, fetchWebResult]);

  const handleOpenDialog = useCallback(() => {
    resetWebStatus();
    clearWebState();
    setDialogOpen(true);
  }, [clearWebState, resetWebStatus]);

  const handleCloseDialog = useCallback(() => {
    setDialogOpen(false);
    clearWebState();
    resetWebStatus();
  }, [clearWebState, resetWebStatus]);

  const handleManualWebCheck = useCallback(() => {
    if (!webFlowId) {
      message.info('Start browser authorization first.');
      return;
    }
    setWebStatus('waiting');
    setWebStatusMessage('Checking authorization status...');
    fetchWebResult(webFlowId);
  }, [fetchWebResult, webFlowId]);

  const handleSubmit = useCallback(async () => {
    if (!clientId.trim() || !clientSecret.trim() || !redirectUri.trim()) {
      message.error(
        'Please fill in Client ID, Client Secret, and Redirect URI.',
      );
      return;
    }

    const trimmedClientId = clientId.trim();
    const trimmedClientSecret = clientSecret.trim();
    const trimmedRedirectUri = redirectUri.trim();

    const payloadForStorage: BoxCredentials = {
      client_id: trimmedClientId,
      client_secret: trimmedClientSecret,
      redirect_uri: trimmedRedirectUri,
    };

    setSubmitLoading(true);
    resetWebStatus();
    clearWebState();

    try {
      const { data } = await startBoxWebAuth({
        client_id: trimmedClientId,
        client_secret: trimmedClientSecret,
        redirect_uri: trimmedRedirectUri,
      });

      if (data.code === 0 && data.data?.authorization_url) {
        onChange(JSON.stringify(payloadForStorage));

        const popup = window.open(
          data.data.authorization_url,
          'ragflow-box-oauth',
          'width=600,height=720',
        );
        if (!popup) {
          message.error(
            'Popup was blocked. Please allow popups for this site.',
          );
          clearWebState();
          return;
        }
        popup.focus();

        const flowId = data.data.flow_id;
        setWebFlowId(flowId);
        webFlowIdRef.current = flowId;
        setWebStatus('waiting');
        setWebStatusMessage(
          'Complete the Box consent in the opened window and return here.',
        );
        message.info(
          'Authorization window opened. Complete the Box consent to continue.',
        );
      } else {
        message.error(data.message || 'Failed to start Box authorization.');
      }
    } catch (_error) {
      message.error('Failed to start Box authorization.');
    } finally {
      setSubmitLoading(false);
    }
  }, [
    clearWebState,
    clientId,
    clientSecret,
    redirectUri,
    resetWebStatus,
    onChange,
  ]);

  return (
    <div className="flex flex-col gap-3">
      {(hasConfigured || hasAuthorized) && (
        <div className="flex flex-wrap items-center gap-3 rounded-md border border-dashed border-muted-foreground/40 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
          <div className="flex flex-wrap items-center gap-2">
            {hasAuthorized ? (
              <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-emerald-700">
                Authorized
              </span>
            ) : null}
            {hasConfigured ? (
              <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-blue-700">
                Configured
              </span>
            ) : null}
          </div>
          <p className="m-0">
            {hasAuthorized
              ? 'Box OAuth credentials are authorized and ready to use.'
              : 'Box OAuth client information has been stored. Run the browser authorization to finalize the setup.'}
          </p>
        </div>
      )}

      <Button variant="outline" onClick={handleOpenDialog}>
        {hasConfigured ? 'Get Box credentials' : 'Configure Box credentials'}
      </Button>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) =>
          !open ? handleCloseDialog() : setDialogOpen(true)
        }
      >
        <DialogContent
          onPointerDownOutside={(e) => e.preventDefault()}
          onInteractOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Configure Box OAuth credentials</DialogTitle>
            <DialogDescription>
              Enter your Box application&apos;s Client ID, Client Secret, and
              Redirect URI. These values will be stored in the form field and
              can be used later to start the OAuth flow.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3 pt-2">
            <div className="space-y-1">
              <label className="text-sm font-medium">Client ID</label>
              <Input
                value={clientId}
                placeholder="Enter Box Client ID"
                onChange={(e) => setClientId(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">Client Secret</label>
              <Input
                type="password"
                value={clientSecret}
                placeholder="Enter Box Client Secret"
                onChange={(e) => setClientSecret(e.target.value)}
              />
            </div>
            <div className="space-y-1">
              <label className="text-sm font-medium">Redirect URI</label>
              <Input
                value={redirectUri}
                placeholder="https://example.com/box/oauth/callback"
                onChange={(e) => setRedirectUri(e.target.value)}
              />
            </div>
            {webStatus !== 'idle' && (
              <div className="rounded-md border border-dashed border-muted-foreground/40 bg-muted/10 px-4 py-4 text-sm text-muted-foreground">
                <div className="text-sm font-semibold text-foreground">
                  Browser authorization
                </div>
                <p
                  className={`mt-2 text-xs ${
                    webStatus === 'error'
                      ? 'text-destructive'
                      : 'text-muted-foreground'
                  }`}
                >
                  {webStatusMessage}
                </p>
                {webStatus === 'waiting' && webFlowId ? (
                  <div className="mt-3 flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleManualWebCheck}
                    >
                      Refresh status
                    </Button>
                  </div>
                ) : null}
              </div>
            )}
          </div>

          <DialogFooter className="pt-3">
            <Button
              variant="ghost"
              onClick={handleCloseDialog}
              disabled={submitLoading}
            >
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={submitLoading}>
              {submitLoading && (
                <Loader2 className="mr-2 size-4 animate-spin" />
              )}
              Submit & Authorize
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default BoxTokenField;
