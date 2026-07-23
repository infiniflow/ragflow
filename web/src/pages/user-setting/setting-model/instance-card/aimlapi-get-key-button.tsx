import { Button } from '@/components/ui/button';
import { useTranslate } from '@/hooks/common-hooks';
import {
  AimlapiAuthorizeStatus,
  useAimlapiAuthorize,
} from '@/hooks/use-aimlapi-authorize';
import { cn } from '@/lib/utils';
import { KeyRound, Loader2 } from 'lucide-react';
import { FC } from 'react';

const STATUS_TEXT_KEY: Partial<Record<AimlapiAuthorizeStatus, string>> = {
  awaiting_consent: 'aimlapiAwaitingConsent',
  success: 'aimlapiKeyAdded',
  denied: 'aimlapiAuthDenied',
  expired: 'aimlapiAuthExpired',
  error: 'aimlapiAuthFailed',
};

/**
 * "Get API key" control for the AIMLAPI provider dialog. Runs the AIMLAPI
 * agent-authorization (OAuth device grant) flow and, on success, hands the
 * issued key back to the modal via `onKey` so it can be filled into the form.
 */
export const AimlapiGetKeyButton: FC<{ onKey: (apiKey: string) => void }> = ({
  onKey,
}) => {
  const { t } = useTranslate('setting');
  const { status, error, start, checkNow } = useAimlapiAuthorize({ onKey });

  const busy = status === 'starting' || status === 'awaiting_consent';
  const statusKey = STATUS_TEXT_KEY[status];

  return (
    <div className="flex flex-col gap-1.5 mb-4">
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={start}
          disabled={busy}
        >
          {busy ? (
            <Loader2 className="animate-spin" size={14} />
          ) : (
            <KeyRound size={14} />
          )}
          {t('aimlapiGetKey')}
        </Button>
        {status === 'awaiting_consent' && (
          <Button type="button" variant="ghost" size="sm" onClick={checkNow}>
            {t('aimlapiCheckStatus')}
          </Button>
        )}
      </div>
      {statusKey && (
        <span
          className={cn('text-xs', {
            'text-state-success': status === 'success',
            'text-text-secondary': status === 'awaiting_consent',
            'text-state-error':
              status === 'denied' || status === 'expired' || status === 'error',
          })}
        >
          {t(statusKey)}
          {status === 'error' && error ? `: ${error}` : ''}
        </span>
      )}
    </div>
  );
};

export default AimlapiGetKeyButton;
