/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { Button } from '@/components/ui/button';
import { useTranslate } from '@/hooks/common-hooks';
import { cn } from '@/lib/utils';
import { replaceText } from '@/pages/dataset/process-log-modal';
import { ApiKeyPostBody } from '@/pages/user-setting/interface';
import { RefreshCcw } from 'lucide-react';
import { memo, useCallback, useState } from 'react';
import { useFormContext } from 'react-hook-form';
import { VerifyResult } from '../hooks';

interface IVerifyButton {
  onVerify: (params: any) => Promise<VerifyResult>;
  isAbsolute?: boolean;
  params?: any;
  className?: string;
  /** Override the success label shown next to the button. Defaults to t('keyValid'). */
  validLabel?: string;
  /** Override the failure label shown next to the button. Defaults to t('keyInvalid'). */
  invalidLabel?: string;
  verifyCallback?: (result: VerifyResult | null) => void;
  /**
   * Optional ref to a form-like object exposing `trigger()` and
   * `getValues()`. Use this when the button is rendered as a *sibling*
   * of the form (i.e. outside any FormProvider). When omitted, falls
   * back to react-hook-form's `useFormContext()`.
   */
  formRef?: {
    current: { trigger: () => Promise<boolean>; getValues: () => any } | null;
  };
}

const VerifyButton: React.FC<IVerifyButton> = ({
  onVerify,
  isAbsolute = true,
  params,
  className,
  validLabel,
  invalidLabel,
  verifyCallback,
  formRef,
}) => {
  const { t, i18n } = useTranslate('setting');
  const isArabic = (i18n.resolvedLanguage || i18n.language || '')
    .toLowerCase()
    .startsWith('ar');
  const [isVerifying, setIsVerifying] = useState(false);
  const [verifyResult, setVerifyResult] = useState<VerifyResult | null>(null);
  const contextForm = useFormContext();

  const onHandleVerify = useCallback(async () => {
    const form = formRef?.current ?? contextForm;
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
      verifyCallback?.(result);
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
      verifyCallback?.({
        isValid: false,
        logs: logs,
      });
    } finally {
      // setVerifyLoading(false);
    }
  }, [formRef, contextForm, onVerify, params, verifyCallback]);
  const handleVerify = useCallback(async () => {
    setVerifyResult({
      isValid: null,
      logs: '',
    });
    setIsVerifying(true);
    try {
      await onHandleVerify();
    } catch (error) {
      const res = {
        isValid: false,
        logs: (error as Error).message || 'Unknown error',
      };
      setVerifyResult(res);
      verifyCallback?.(res);
    } finally {
      setIsVerifying(false);
    }
  }, [onHandleVerify, verifyCallback]);

  return (
    <div
      className={cn(
        !isAbsolute || (verifyResult && verifyResult.isValid === false)
          ? 'flex flex-col gap-5 w-full '
          : `absolute bottom-6 z-[100] ${isArabic ? 'right-6' : 'left-6'}`,
        className,
      )}
    >
      <div className="flex gap-2 items-center">
        <Button
          type="button"
          onClick={handleVerify}
          disabled={isVerifying}
          variant={'outline'}
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
              {verifyResult.isValid
                ? (validLabel ?? t('keyValid'))
                : (invalidLabel ?? t('keyInvalid'))}
            </span>
          </div>
        )}
      </div>
      {verifyResult && verifyResult.isValid === false && verifyResult.logs && (
        <div className="space-y-2">
          <div className="w-full  whitespace-pre-line text-wrap bg-bg-card rounded-lg h-fit max-h-[250px] overflow-y-auto scrollbar-auto p-2.5">
            {replaceText(verifyResult.logs)}
          </div>
        </div>
      )}
    </div>
  );
};

export default memo(VerifyButton);
