import HighLightMarkdown from '@/components/highlight-markdown';
import message from '@/components/ui/message';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
} from '@/constants/common';
import { useTranslate } from '@/hooks/common-hooks';
import { useCallback, useMemo, useState } from 'react';

type IEmbedAppModalProps = {
  open: any;
  url: string;
  token: string;
  from: string;
  setOpen: (e: any) => void;
  tenantId: string;
  beta?: string;
};

const EmbedAppModal = (props: IEmbedAppModalProps) => {
  const { t } = useTranslate('search');
  const { open, setOpen, token = '', from, url, tenantId, beta = '' } = props;

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
            placeholder="Select a locale"
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
            <button
              type="button"
              onClick={() => {
                navigator.clipboard.writeText(token);
                message.success(t('copySuccess'));
              }}
              className="ml-2 p-2 hover:text-white transition-colors"
              title="Copy ID"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                className="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={2}
                  d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h10a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </Modal>
  );
};
export default EmbedAppModal;
