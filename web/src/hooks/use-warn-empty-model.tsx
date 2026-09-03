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

import { Modal } from '@/components/ui/modal/modal';
import DOMPurify from 'dompurify';
import { isEmpty } from 'lodash';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigatePage } from './logic-hooks/navigate-hooks';

let isWarningVisible = false;

export const useWarnEmptyModel = (
  showEmptyModelWarn: boolean,
  embdId?: string,
  llmId?: string,
  loading?: boolean,
) => {
  const { t } = useTranslation();
  const warnedRef = useRef(false);
  const { navigateToModelSetting } = useNavigatePage();

  useEffect(() => {
    if (
      showEmptyModelWarn &&
      !warnedRef.current &&
      !isWarningVisible &&
      !loading &&
      (isEmpty(embdId) || isEmpty(llmId)) &&
      typeof embdId === 'string' &&
      typeof llmId === 'string'
    ) {
      warnedRef.current = true;
      isWarningVisible = true;
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
          isWarningVisible = false;
          navigateToModelSetting();
        },
      });
    }
  }, [showEmptyModelWarn, embdId, llmId, loading, navigateToModelSetting, t]);
};
