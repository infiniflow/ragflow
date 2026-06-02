import { Modal } from '@/components/ui/modal/modal';
import DOMPurify from 'dompurify';
import { isEmpty } from 'lodash';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigatePage } from './logic-hooks/navigate-hooks';

export const useWarnEmptyModel = (
  showEmptyModelWarn: boolean,
  embdId?: string,
  llmId?: string,
) => {
  const { t } = useTranslation();
  const warnedRef = useRef(false);
  const { navigateToModelSetting } = useNavigatePage();

  useEffect(() => {
    if (
      showEmptyModelWarn &&
      !warnedRef.current &&
      (isEmpty(embdId) || isEmpty(llmId)) &&
      typeof embdId === 'string' &&
      typeof llmId === 'string'
    ) {
      warnedRef.current = true;
      Modal.warning({
        title: t('common.warn'),
        content: (
          <div
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(t('setting.modelProvidersWarn')),
            }}
          ></div>
        ),
        closable: false,
        showCancel: false,
        onOk() {
          navigateToModelSetting();
        },
      });
    }
  }, [showEmptyModelWarn, embdId, llmId, navigateToModelSetting, t]);
};
