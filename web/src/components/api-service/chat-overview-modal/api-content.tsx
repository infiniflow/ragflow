import { useFetchNextStats } from '@/hooks/chat-hooks';
import { useSetModalState, useTranslate } from '@/hooks/common-hooks';
import { Button, Card, DatePicker, Flex, Space } from 'antd';
import { RangePickerProps } from 'antd/es/date-picker';
import dayjs from 'dayjs';
import ChatApiKeyModal from '../chat-api-key-modal';
import EmbedModal from '../embed-modal';
import { usePreviewChat, useShowEmbedModal } from '../hooks';
import BackendServiceApi from './backend-service-api';
import StatsChart from './stats-chart';

const { RangePicker } = DatePicker;

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

  const { pickerValue, setPickerValue } = useFetchNextStats();

  const disabledDate: RangePickerProps['disabledDate'] = (current) => {
    return current && current > dayjs().endOf('day');
  };

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

        <Space>
          <b>{t('dateRange')}</b>
          <RangePicker
            disabledDate={disabledDate}
            value={pickerValue}
            onChange={setPickerValue}
            allowClear={false}
          />
        </Space>
        <StatsChart></StatsChart>
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
