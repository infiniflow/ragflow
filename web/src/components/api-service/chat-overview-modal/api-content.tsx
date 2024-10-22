import HightLightMarkdown from '@/components/highlight-markdown';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { Button, Card, Flex, Space } from 'antd';
// import apiDoc from '../../../../../api/http_api_reference.md';
import apiDoc from '@parent/api/http_api_reference.md';
import ChatApiKeyModal from '../chat-api-key-modal';
import EmbedModal from '../embed-modal';
import { usePreviewChat, useShowEmbedModal } from '../hooks';
import BackendServiceApi from './backend-service-api';

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
  const { embedVisible, hideEmbedModal, showEmbedModal, embedToken } =
    useShowEmbedModal(idKey, id);

  const { handlePreview } = usePreviewChat(idKey, id);

  return (
    <div>
      <Flex vertical gap={'middle'}>
        <BackendServiceApi show={showApiKeyModal}></BackendServiceApi>
        {!hideChatPreviewCard && (
          <Card title={`${name} Web App`}>
            <Flex gap={8} vertical>
              <Space size={'middle'}>
                <Button onClick={handlePreview}>{t('preview')}</Button>
                <Button onClick={showEmbedModal}>{t('embedded')}</Button>
              </Space>
            </Flex>
          </Card>
        )}
        <HightLightMarkdown>{apiDoc}</HightLightMarkdown>
      </Flex>
      {apiKeyVisible && (
        <ChatApiKeyModal
          hideModal={hideApiKeyModal}
          dialogId={id}
          idKey={idKey}
        ></ChatApiKeyModal>
      )}
      {embedVisible && (
        <EmbedModal
          token={embedToken}
          visible={embedVisible}
          hideModal={hideEmbedModal}
        ></EmbedModal>
      )}
    </div>
  );
};

export default ApiContent;
