import { useIsDarkTheme } from '@/components/theme-provider';
import { useSetModalState } from '@/hooks/common-hooks';
import { LangfuseCard } from '@/pages/user-setting/setting-model/langfuse';
import apiDoc from '@parent/docs/references/http_api_reference.md?raw';
import MarkdownPreview from '@uiw/react-markdown-preview';
import ChatApiKeyModal from '../chat-api-key-modal';
import BackendServiceApi from './backend-service-api';
import MarkdownToc from './markdown-toc';

const ApiContent = ({ id, idKey }: { id?: string; idKey: string }) => {
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const isDarkTheme = useIsDarkTheme();

  return (
    <div className="pb-2">
      <section className="flex flex-col gap-2 pb-5">
        <BackendServiceApi show={showApiKeyModal}></BackendServiceApi>

        <div style={{ position: 'relative' }}>
          <MarkdownToc content={apiDoc} />
        </div>
        <MarkdownPreview
          source={apiDoc}
          wrapperElement={{ 'data-color-mode': isDarkTheme ? 'dark' : 'light' }}
        ></MarkdownPreview>
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
