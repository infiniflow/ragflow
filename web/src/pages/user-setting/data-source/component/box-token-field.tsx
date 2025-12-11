import { useCallback, useEffect, useMemo, useState } from 'react';

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

export type BoxTokenFieldProps = {
  /** 存储在表单里的值，约定为 JSON 字符串 */
  value?: string;
  /** 表单回写，用 JSON 字符串承载 client_id / client_secret / redirect_uri */
  onChange: (value: any) => void;
  placeholder?: string;
};

type BoxCredentials = {
  client_id?: string;
  client_secret?: string;
  redirect_uri?: string;
};

const parseBoxCredentials = (content?: string): BoxCredentials | null => {
  if (!content) return null;
  try {
    const parsed = JSON.parse(content);
    return {
      client_id: parsed.client_id,
      client_secret: parsed.client_secret,
      redirect_uri: parsed.redirect_uri,
    };
  } catch {
    return null;
  }
};

const BoxTokenField = ({
  value,
  onChange,
  placeholder,
}: BoxTokenFieldProps) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [redirectUri, setRedirectUri] = useState('');

  const parsed = useMemo(() => parseBoxCredentials(value), [value]);

  // 当外部 value 变化且弹窗关闭时，同步初始值
  useEffect(() => {
    if (!dialogOpen) {
      setClientId(parsed?.client_id ?? '');
      setClientSecret(parsed?.client_secret ?? '');
      setRedirectUri(parsed?.redirect_uri ?? '');
    }
  }, [parsed, dialogOpen]);

  const hasConfigured = useMemo(
    () =>
      Boolean(
        parsed?.client_id && parsed?.client_secret && parsed?.redirect_uri,
      ),
    [parsed],
  );

  const handleOpenDialog = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const handleCloseDialog = useCallback(() => {
    setDialogOpen(false);
  }, []);

  const handleSubmit = useCallback(() => {
    if (!clientId.trim() || !clientSecret.trim() || !redirectUri.trim()) {
      message.error(
        'Please fill in Client ID, Client Secret, and Redirect URI.',
      );
      return;
    }

    const payload: BoxCredentials = {
      client_id: clientId.trim(),
      client_secret: clientSecret.trim(),
      redirect_uri: redirectUri.trim(),
    };

    try {
      onChange(JSON.stringify(payload));
      message.success('Box credentials saved locally.');
      setDialogOpen(false);
    } catch {
      message.error('Failed to save Box credentials.');
    }
  }, [clientId, clientSecret, redirectUri, onChange]);

  return (
    <div className="flex flex-col gap-3">
      {hasConfigured && (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-dashed border-muted-foreground/40 bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
          <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-emerald-700">
            Configured
          </span>
          <p className="m-0">
            Box OAuth credentials have been configured. You can update them at
            any time.
          </p>
        </div>
      )}

      <Button variant="outline" onClick={handleOpenDialog}>
        {hasConfigured ? 'Edit Box credentials' : 'Configure Box credentials'}
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
                placeholder={placeholder || 'Enter Box Client ID'}
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
          </div>

          <DialogFooter className="pt-3">
            <Button variant="ghost" onClick={handleCloseDialog}>
              Cancel
            </Button>
            <Button onClick={handleSubmit}>Submit</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default BoxTokenField;
