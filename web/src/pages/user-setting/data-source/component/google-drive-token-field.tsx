import { FileUploader } from '@/components/file-uploader';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import message from '@/components/ui/message';
import { FileMimeType } from '@/constants/common';
import {
  pollGoogleDriveWebAuthResult,
  startGoogleDriveWebAuth,
} from '@/services/data-source-service';
import { Loader2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

type GoogleDriveTokenFieldProps = {
  value?: string;
  onChange: (value: any) => void;
};

const credentialHasRefreshToken = (content: string) => {
  try {
    const parsed = JSON.parse(content);
    return Boolean(parsed?.refresh_token);
  } catch {
    return false;
  }
};

const describeCredentials = (content?: string) => {
  if (!content) return '';
  try {
    const parsed = JSON.parse(content);
    if (parsed?.refresh_token) {
      return 'Uploaded OAuth tokens with a refresh token.';
    }
    if (parsed?.installed || parsed?.web) {
      return 'Client credentials detected. Complete verification to mint long-lived tokens.';
    }
    return 'Stored Google credential JSON.';
  } catch {
    return '';
  }
};

const GoogleDriveTokenField = ({
  value,
  onChange,
}: GoogleDriveTokenFieldProps) => {
  const [files, setFiles] = useState<File[]>([]);
  const [pendingCredentials, setPendingCredentials] = useState<string>('');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [webAuthLoading, setWebAuthLoading] = useState(false);
  const [webFlowId, setWebFlowId] = useState<string | null>(null);
  const [webStatus, setWebStatus] = useState<
    'idle' | 'waiting' | 'success' | 'error'
  >('idle');
  const [webStatusMessage, setWebStatusMessage] = useState('');
  const webFlowIdRef = useRef<string | null>(null);
  const webPollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearWebState = useCallback(() => {
    if (webPollTimerRef.current) {
      clearTimeout(webPollTimerRef.current);
      webPollTimerRef.current = null;
    }
    webFlowIdRef.current = null;
    setWebFlowId(null);
    setWebStatus('idle');
    setWebStatusMessage('');
  }, []);

  useEffect(() => {
    return () => {
      if (webPollTimerRef.current) {
        clearTimeout(webPollTimerRef.current);
      }
    };
  }, []);

  useEffect(() => {
    webFlowIdRef.current = webFlowId;
  }, [webFlowId]);

  const credentialSummary = useMemo(() => describeCredentials(value), [value]);
  const hasVerifiedTokens = useMemo(
    () => Boolean(value && credentialHasRefreshToken(value)),
    [value],
  );
  const hasUploadedButUnverified = useMemo(
    () => Boolean(value && !hasVerifiedTokens),
    [hasVerifiedTokens, value],
  );

  const resetDialog = useCallback(
    (shouldResetState: boolean) => {
      setDialogOpen(false);
      clearWebState();
      if (shouldResetState) {
        setPendingCredentials('');
        setFiles([]);
      }
    },
    [clearWebState],
  );

  const fetchWebResult = useCallback(
    async (flowId: string) => {
      try {
        const { data } = await pollGoogleDriveWebAuthResult({
          flow_id: flowId,
        });
        if (data.code === 0 && data.data?.credentials) {
          onChange(data.data.credentials);
          setPendingCredentials('');
          message.success('Google Drive credentials verified.');
          resetDialog(false);
          return;
        }
        if (data.code === 106) {
          setWebStatus('waiting');
          setWebStatusMessage('Authorization confirmed. Finalizing tokens...');
          if (webPollTimerRef.current) {
            clearTimeout(webPollTimerRef.current);
          }
          webPollTimerRef.current = setTimeout(
            () => fetchWebResult(flowId),
            1500,
          );
          return;
        }
        message.error(data.message || 'Authorization failed.');
        clearWebState();
      } catch (err) {
        message.error('Unable to retrieve authorization result.');
        clearWebState();
      }
    },
    [clearWebState, onChange, resetDialog],
  );

  useEffect(() => {
    const handler = (event: MessageEvent) => {
      const payload = event.data;
      if (!payload || payload.type !== 'ragflow-google-drive-oauth') {
        return;
      }
      if (!payload.flowId) {
        return;
      }
      if (webFlowIdRef.current && webFlowIdRef.current !== payload.flowId) {
        return;
      }

      if (payload.status === 'success') {
        setWebStatus('waiting');
        setWebStatusMessage('Authorization confirmed. Finalizing tokens...');
        fetchWebResult(payload.flowId);
      } else {
        message.error(
          payload.message || 'Authorization window reported an error.',
        );
        clearWebState();
      }
    };

    window.addEventListener('message', handler);
    return () => window.removeEventListener('message', handler);
  }, [clearWebState, fetchWebResult]);

  const handleValueChange = useCallback(
    (nextFiles: File[]) => {
      if (!nextFiles.length) {
        setFiles([]);
        onChange('');
        setPendingCredentials('');
        clearWebState();
        return;
      }
      const file = nextFiles[nextFiles.length - 1];
      file
        .text()
        .then((text) => {
          try {
            JSON.parse(text);
          } catch {
            message.error('Invalid JSON file.');
            setFiles([]);
            clearWebState();
            return;
          }
          setFiles([file]);
          clearWebState();
          if (credentialHasRefreshToken(text)) {
            onChange(text);
            setPendingCredentials('');
            message.success('OAuth credentials uploaded.');
            return;
          }
          setPendingCredentials(text);
          setDialogOpen(true);
          message.info(
            'Client configuration uploaded. Verification is required to finish setup.',
          );
        })
        .catch(() => {
          message.error('Unable to read the uploaded file.');
          setFiles([]);
        });
    },
    [clearWebState, onChange],
  );

  const handleStartWebAuthorization = useCallback(async () => {
    if (!pendingCredentials) {
      message.error('No Google credential file detected.');
      return;
    }
    setWebAuthLoading(true);
    clearWebState();
    try {
      const { data } = await startGoogleDriveWebAuth({
        credentials: pendingCredentials,
      });
      if (data.code === 0 && data.data?.authorization_url) {
        const flowId = data.data.flow_id;
        const popup = window.open(
          data.data.authorization_url,
          'ragflow-google-drive-oauth',
          'width=600,height=720',
        );
        if (!popup) {
          message.error(
            'Popup was blocked. Please allow popups for this site.',
          );
          return;
        }
        popup.focus();
        webFlowIdRef.current = flowId;
        setWebFlowId(flowId);
        setWebStatus('waiting');
        setWebStatusMessage('Complete the Google consent in the popup window.');
      } else {
        message.error(data.message || 'Failed to start browser authorization.');
      }
    } catch (err) {
      message.error('Failed to start browser authorization.');
    } finally {
      setWebAuthLoading(false);
    }
  }, [clearWebState, pendingCredentials]);

  const handleManualWebCheck = useCallback(() => {
    if (!webFlowId) {
      message.info('Start browser authorization first.');
      return;
    }
    setWebStatus('waiting');
    setWebStatusMessage('Checking authorization status...');
    fetchWebResult(webFlowId);
  }, [fetchWebResult, webFlowId]);

  const handleCancel = useCallback(() => {
    message.warning(
      'Verification canceled. Upload the credential again to restart.',
    );
    resetDialog(true);
  }, [resetDialog]);

  return (
    <div className="flex flex-col gap-3">
      {(credentialSummary ||
        hasVerifiedTokens ||
        hasUploadedButUnverified ||
        pendingCredentials) && (
        <div className="flex flex-wrap items-center gap-3 rounded-md border border-dashed border-muted-foreground/40 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
          <div className="flex flex-wrap items-center gap-2">
            {hasVerifiedTokens ? (
              <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-emerald-700">
                Verified
              </span>
            ) : null}
            {hasUploadedButUnverified ? (
              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-amber-700">
                Needs authorization
              </span>
            ) : null}
            {pendingCredentials && !hasVerifiedTokens ? (
              <span className="rounded-full bg-blue-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-blue-700">
                Uploaded (pending)
              </span>
            ) : null}
          </div>
          {credentialSummary ? (
            <p className="m-0">{credentialSummary}</p>
          ) : null}
        </div>
      )}
      <FileUploader
        className="py-4 border-[0.5px] bg-bg-card text-text-secondary"
        value={files}
        onValueChange={handleValueChange}
        accept={{ '*.json': [FileMimeType.Json] }}
        maxFileCount={1}
        description="Upload your Google OAuth JSON file."
      />

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!open && dialogOpen) {
            handleCancel();
          }
        }}
      >
        <DialogContent
          onPointerDownOutside={(e) => e.preventDefault()}
          onInteractOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        >
          <DialogHeader>
            <DialogTitle>Complete Google verification</DialogTitle>
            <DialogDescription>
              The uploaded client credentials do not contain a refresh token.
              Run the verification flow once to mint reusable tokens.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="rounded-md border border-dashed border-muted-foreground/40 bg-muted/10 px-4 py-4 text-sm text-muted-foreground">
              <div className="text-sm font-semibold text-foreground">
                Authorize in browser
              </div>
              <p className="mt-2">
                We will open Google&apos;s consent page in a new window. Sign in
                with the admin account, grant access, and return here. Your
                credentials will update automatically.
              </p>
              {webStatus !== 'idle' && (
                <p
                  className={`mt-2 text-xs ${
                    webStatus === 'error'
                      ? 'text-destructive'
                      : 'text-muted-foreground'
                  }`}
                >
                  {webStatusMessage}
                </p>
              )}
              <div className="mt-3 flex flex-wrap gap-2">
                <Button
                  onClick={handleStartWebAuthorization}
                  disabled={webAuthLoading}
                >
                  {webAuthLoading && (
                    <Loader2 className="mr-2 size-4 animate-spin" />
                  )}
                  Authorize with Google
                </Button>
                {webFlowId ? (
                  <Button
                    variant="outline"
                    onClick={handleManualWebCheck}
                    disabled={webStatus === 'success'}
                  >
                    Refresh status
                  </Button>
                ) : null}
              </div>
            </div>
          </div>
          <DialogFooter className="pt-2">
            <Button variant="ghost" onClick={handleCancel}>
              Cancel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default GoogleDriveTokenField;
