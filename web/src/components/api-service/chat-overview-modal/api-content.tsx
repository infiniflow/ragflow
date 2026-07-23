import { useIsDarkTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { LangfuseCard } from '@/pages/user-setting/setting-model/langfuse';
import apiDoc from '@parent/docs/references/http_api_reference.md?raw';
import { Loader2 } from 'lucide-react';
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import ChatApiKeyModal from '../chat-api-key-modal';
import BackendServiceApi from './backend-service-api';
import MarkdownToc from './markdown-toc';

const LazyMarkdownPreview = lazy(() => import('@uiw/react-markdown-preview'));

const removeFrontmatter = (content: string): string => {
  const lines = content.split('\n');
  if (lines[0]?.trim() === '---') {
    const endIndex = lines.slice(1).findIndex((line) => line.trim() === '---');
    if (endIndex !== -1) {
      return lines.slice(endIndex + 2).join('\n');
    }
  }
  return content;
};

const ApiContent = ({ id, idKey }: { id?: string; idKey: string }) => {
  const { t } = useTranslate('setting');

  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const {
    visible: tocVisible,
    hideModal: hideToc,
    showModal: showToc,
  } = useSetModalState();

  const isDarkTheme = useIsDarkTheme();

  const cleanDoc = useMemo(() => removeFrontmatter(apiDoc), []);

  // Defer the heavy 230KB markdown rendering so the page paints immediately.
  const [docReady, setDocReady] = useState(false);
  useEffect(() => {
    const id = requestAnimationFrame(() => setDocReady(true));
    return () => cancelAnimationFrame(id);
  }, []);

  return (
    <div className="flex flex-col w-full">
      <BackendServiceApi show={showApiKeyModal} />

      <div className="text-left py-4">
        <Button onClick={tocVisible ? hideToc : showToc}>
          {tocVisible ? t('hideToc') : t('showToc')}
        </Button>
      </div>
      <section className="flex flex-col gap-2 pb-5 flex-1 min-h-0 overflow-auto mb-4">
        <div style={{ position: 'relative' }}>
          {tocVisible && <MarkdownToc content={cleanDoc} />}
        </div>
        {docReady ? (
          <Suspense
            fallback={
              <div className="flex justify-center py-10">
                <Loader2 className="size-5 animate-spin text-text-secondary" />
              </div>
            }
          >
            <LazyMarkdownPreview
              source={cleanDoc}
              wrapperElement={{
                'data-color-mode': isDarkTheme ? 'dark' : 'light',
              }}
            />
          </Suspense>
        ) : (
          <div className="flex justify-center py-10">
            <Loader2 className="size-5 animate-spin text-text-secondary" />
          </div>
        )}
      </section>
      <LangfuseCard></LangfuseCard>
      {apiKeyVisible && (
        <ChatApiKeyModal
          hideModal={hideApiKeyModal}
          dialogId={id}
          idKey={idKey}
        ></ChatApiKeyModal>
      )}
    </div>
  );
};

export default ApiContent;
