import CopyToClipboard from '@/components/copy-to-clipboard';
import HightLightMarkdown from '@/components/highlight-markdown';
import {
  UnderlineTabs,
  UnderlineTabsContent,
  UnderlineTabsList,
  UnderlineTabsTrigger,
} from '@/components/originui/underline-tabs';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SharedFrom } from '@/constants/chat';
import {
  LanguageAbbreviation,
  LanguageAbbreviationMap,
} from '@/constants/common';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { memo, useMemo, useState } from 'react';

type IProps = IModalProps<any> & {
  token: string;
  form: SharedFrom;
  beta: string;
  isAgent: boolean;
};

function EmbedDialog({
  hideModal,
  token = '',
  form,
  beta = '',
  isAgent,
}: IProps) {
  const { t } = useTranslate('chat');

  const [visibleAvatar, setVisibleAvatar] = useState(false);
  const [locale, setLocale] = useState('');

  const languageOptions = useMemo(() => {
    return Object.values(LanguageAbbreviation).map((x) => ({
      label: LanguageAbbreviationMap[x],
      value: x,
    }));
  }, []);

  const generateIframeSrc = () => {
    let src = `${location.origin}/chat/share?shared_id=${token}&from=${form}&auth=${beta}`;
    if (visibleAvatar) {
      src += '&visible_avatar=1';
    }
    if (locale) {
      src += `&locale=${locale}`;
    }
    return src;
  };

  const iframeSrc = generateIframeSrc();

  const text = `
  ~~~ html
  <iframe
  src="${iframeSrc}"
  style="width: 100%; height: 100%; min-height: 600px"
  frameborder="0"
>
</iframe>
~~~
  `;

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t('embedIntoSite', { keyPrefix: 'common' })}
          </DialogTitle>
        </DialogHeader>
        <section className="w-full overflow-auto">
          <UnderlineTabs defaultValue="1" className="w-full">
            <UnderlineTabsList>
              <UnderlineTabsTrigger value="1">
                {t('fullScreenTitle')}
              </UnderlineTabsTrigger>
              <UnderlineTabsTrigger value="2">
                {t('partialTitle')}
              </UnderlineTabsTrigger>
              <UnderlineTabsTrigger value="3">
                {t('extensionTitle')}
              </UnderlineTabsTrigger>
            </UnderlineTabsList>
            <UnderlineTabsContent value="1">
              <section>
                <HightLightMarkdown>{text}</HightLightMarkdown>
              </section>
            </UnderlineTabsContent>
            <UnderlineTabsContent value="2">
              {t('comingSoon')}
            </UnderlineTabsContent>
            <UnderlineTabsContent value="3">
              {t('comingSoon')}
            </UnderlineTabsContent>
          </UnderlineTabs>
          <div className="text-base font-medium mt-4 mb-1">
            {t(isAgent ? 'flow' : 'chat', { keyPrefix: 'header' })}
            <span className="ml-1 inline-block">ID</span>
          </div>
          <div className="bg-background-card rounded-md p-2 ">
            {token} <CopyToClipboard text={token}></CopyToClipboard>
          </div>
          <a
            className="pt-3 cursor-pointer text-background-checked inline-block"
            href={
              isAgent
                ? 'https://ragflow.io/docs/dev/http_api_reference#create-session-with-agent'
                : 'https://ragflow.io/docs/dev/http_api_reference#create-session-with-chat-assistant'
            }
            target="_blank"
            rel="noreferrer"
          >
            {t('howUseId', { keyPrefix: isAgent ? 'flow' : 'chat' })}
          </a>
        </section>
      </DialogContent>
    </Dialog>
  );
}

export default memo(EmbedDialog);
