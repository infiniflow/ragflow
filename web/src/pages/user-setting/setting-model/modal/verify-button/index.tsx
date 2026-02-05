import { Button } from '@/components/ui/button';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { replaceText } from '@/pages/dataset/process-log-modal';
import { ApiKeyPostBody } from '@/pages/user-setting/interface';
import { RefreshCcw } from 'lucide-react';
import { memo, useCallback, useState } from 'react';
import { useFormContext } from 'react-hook-form';
import { VerifyResult } from '../../hooks';

interface IVerifyButton {
  onVerify: (params: any) => Promise<VerifyResult>;
  isAbsolute?: boolean;
  params?: any;
}

const VerifyButton: React.FC<IVerifyButton> = ({
  onVerify,
  isAbsolute = true,
  params,
}) => {
  const { t } = useTranslate('setting');
  const [isVerifying, setIsVerifying] = useState(false);
  const [verifyResult, setVerifyResult] = useState<VerifyResult | null>(null);
  const form = useFormContext();

  const onHandleVerify = useCallback(async () => {
    const formValid = await form?.trigger();
    if (!formValid) {
      return;
    }
    // setVerifyLoading(true);
    try {
      const values = form.getValues();
      const result = await onVerify({
        ...values,
        verify: true,
        ...params,
      } as ApiKeyPostBody & { verify: boolean });
      setVerifyResult(result);
    } catch (error: any) {
      let logs = '';

      if (error?.message) {
        logs = error.message;
      } else if (typeof error === 'string') {
        logs = error;
      }

      setVerifyResult({
        isValid: false,
        logs: logs,
      });
    } finally {
      // setVerifyLoading(false);
    }
  }, [form, onVerify, params]);
  const handleVerify = async () => {
    setVerifyResult({
      isValid: null,
      logs: '',
    });
    setIsVerifying(true);
    try {
      await onHandleVerify();
    } catch (error) {
      setVerifyResult({
        isValid: false,
        logs: (error as Error).message || 'Unknown error',
      });
    } finally {
      setIsVerifying(false);
    }
  };

  return (
    <div
      className={cn(
        !isAbsolute || (verifyResult && verifyResult.isValid === false)
          ? 'flex flex-col gap-5 w-full '
          : 'absolute left-6 bottom-6 z-[100]',
      )}
    >
      <div className="flex gap-2 items-center">
        <Button
          type="button"
          onClick={handleVerify}
          disabled={isVerifying}
          variant={'ghost'}
        >
          <RefreshCcw
            size={14}
            className={cn(isVerifying ? 'animate-spin-reverse' : '', '')}
          />
          {t('Verify')}
        </Button>

        {verifyResult && verifyResult.isValid !== null && (
          <div
            className={`flex items-center gap-2 ${
              verifyResult.isValid ? 'text-state-success' : 'text-state-error'
            }`}
          >
            <span>
              {verifyResult.isValid ? t('keyValid') : t('keyInvalid')}
            </span>
          </div>
        )}
      </div>
      {verifyResult && verifyResult.isValid !== null && (
        <div className="space-y-2">
          {verifyResult.logs && (
            <div className="w-full  whitespace-pre-line text-wrap bg-bg-card rounded-lg h-fit max-h-[250px] overflow-y-auto scrollbar-auto p-2.5">
              {replaceText(verifyResult.logs)}
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default memo(VerifyButton);
