import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { LangfuseCard } from '@/pages/user-setting/setting-model/langfuse';
import apiDoc from '@parent/docs/references/http_api_reference.md';
import MarkdownPreview from '@uiw/react-markdown-preview';
import { Button, Card, Flex, Space } from 'antd';
import ChatApiKeyModal from '../chat-api-key-modal';
import { usePreviewChat } from '../hooks';
import BackendServiceApi from './backend-service-api';
import MarkdownToc from './markdown-toc';

const ApiContent = ({
  id,
  idKey,
  hideChatPreviewCard = false,
}: {
  id?: string;
  idKey: string;
  hideChatPreviewCard?: boolean;
}) => {
  const { t } = useTranslate('chat');
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();
  // const { embedVisible, hideEmbedModal, showEmbedModal, embedToken } =
  //   useShowEmbedModal(idKey);

  const { handlePreview } = usePreviewChat(idKey);

  return (
    <div className="pb-2">
      <Flex vertical gap={'middle'}>
        <BackendServiceApi show={showApiKeyModal}></BackendServiceApi>
        {!hideChatPreviewCard && (
          <Card title={`${name} Web App`}>
            <Flex gap={8} vertical>
              <Space size={'middle'}>
                <Button onClick={handlePreview}>{t('preview')}</Button>
                {/* <Button onClick={() => showEmbedModal(id)}>
                  {t('embedded')}
                </Button> */}
              </Space>
            </Flex>
          </Card>
        )}
        <div style={{ position: 'relative' }}>
          <MarkdownToc content={apiDoc} />
        </div>
        <MarkdownPreview source={apiDoc}></MarkdownPreview>
      </Flex>
      {apiKeyVisible && (
        <ChatApiKeyModal
          hideModal={hideApiKeyModal}
          dialogId={id}
          idKey={idKey}
        ></ChatApiKeyModal>
      )}
      {/* {embedVisible && (
        <EmbedModal
          token={embedToken}
          visible={embedVisible}
          hideModal={hideEmbedModal}
        ></EmbedModal>
      )} */}
      <LangfuseCard></LangfuseCard>
    </div>
  );
};

export default ApiContent;
