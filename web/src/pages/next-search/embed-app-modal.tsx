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

import HighLightMarkdown from '@/components/highlight-markdown';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
} from '@/constants/common';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { ExternalLink } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import CopyToClipboard from '@/components/copy-to-clipboard';

type IEmbedAppModalProps = {
  open: any;
  url: string;
  token: string;
  from: string;
  setOpen: (e: any) => void;
  beta?: string;
};

const EmbedAppModal = (props: IEmbedAppModalProps) => {
  const { t } = useTranslate('search');
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const { open, setOpen, token = '', from, url, beta = '' } = props;

  const [hideAvatar, setHideAvatar] = useState(false);
  const [locale, setLocale] = useState('');

  const languageOptions = useMemo(() => {
    return Object.values(LanguageAbbreviation).map((x) => ({
      label: LanguageAbbreviationMap[x],
      value: x,
    }));
  }, []);

  const generateIframeSrc = useCallback(() => {
    // const { visibleAvatar, locale } = values;
    let src = `${location.origin}${url}?shared_id=${token}&from=${from}&auth=${beta}&tenantId=${tenantId}`;
    if (hideAvatar) {
      src += '&visible_avatar=1';
    }
    if (locale) {
      src += `&locale=${locale}`;
    }
    return src;
  }, [beta, from, token, hideAvatar, locale, url, tenantId]);

  const handleOpenInNewTab = useCallback(() => {
    window.open(generateIframeSrc(), '_blank');
  }, [generateIframeSrc]);

  // ... existing code ...
  const text = useMemo(() => {
    const iframeSrc = generateIframeSrc();
    return `\`\`\`html
<iframe
  src="${iframeSrc}"
  style="width: 100%; height: 100%; min-height: 600px"
  frameborder="0">
</iframe>
\`\`\``;
  }, [generateIframeSrc]);
  // ... existing code ...
  return (
    <Modal
      title={t('embedIntoSite', { keyPrefix: 'common' })}
      className="!bg-bg-base !text-text-disabled"
      open={open}
      onCancel={() => setOpen(false)}
      showfooter={false}
      footer={null}
    >
      <div className="w-full">
        {/* Hide Avatar Toggle */}
        <div className="mb-6">
          <label className="block text-sm font-medium mb-2">
            {t('profile')}
          </label>
          <div className="flex items-center">
            <Switch
              checked={hideAvatar}
              onCheckedChange={(value) => {
                setHideAvatar(value);
              }}
            />
          </div>
        </div>

        {/* Locale Select */}
        <div className="mb-6">
          <label className="block text-sm font-medium mb-2">
            {t('locale')}
          </label>
          <RAGFlowSelect
            placeholder={t('selectLocalePlaceholder')}
            value={locale}
            onChange={(value) => setLocale(value)}
            options={languageOptions}
          ></RAGFlowSelect>
        </div>
        {/* Embed Code */}
        <div className="mb-6">
          <label className="block text-sm font-medium mb-2">
            {t('embedCode')}
          </label>
          {/* <div className=" border rounded-lg"> */}
          {/* <pre className="text-sm whitespace-pre-wrap">{text}</pre> */}
          <HighLightMarkdown>{text}</HighLightMarkdown>
          {/* </div> */}
        </div>

        {/* Open In New Tab */}
        <div className="mb-6">
          <Button
            onClick={handleOpenInNewTab}
            className="w-full"
            variant="secondary"
          >
            <ExternalLink className="mr-2 h-4 w-4" />
            {t('openInNewTab', { keyPrefix: 'common' })}
          </Button>
        </div>

        {/* ID Field */}
        <div className="mb-4">
          <label className="block text-sm font-medium mb-2">{t('id')}</label>
          <div className="flex items-center border border-border rounded-lg bg-bg-base">
            <input
              type="text"
              value={token}
              readOnly
              className="flex-1 px-4 py-2 focus:outline-none bg-bg-base rounded-lg"
            />
            <CopyToClipboard text={token}></CopyToClipboard>
          </div>
        </div>
      </div>
    </Modal>
  );
};
export default EmbedAppModal;
