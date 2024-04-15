import LineChart from '@/components/line-chart';
import { useSetModalState, useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { Button, Card, DatePicker, Flex, Modal, Space, Typography } from 'antd';
import dayjs from 'dayjs';
import ChatApiKeyModal from '../chat-api-key-modal';

import { RangePickerProps } from 'antd/es/date-picker';
import { useFetchStatsOnMount } from '../hooks';
import styles from './index.less';

const { Paragraph } = Typography;
const { RangePicker } = DatePicker;

const ChatOverviewModal = ({
  visible,
  hideModal,
}: IModalProps<any> & { dialogId: string }) => {
  const { t } = useTranslate('chat');

  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();

  const { pickerValue, setPickerValue } = useFetchStatsOnMount(visible);

  const disabledDate: RangePickerProps['disabledDate'] = (current) => {
    return current && current > dayjs().endOf('day');
  };

  return (
    <>
      <Modal
        title={t('overview')}
        open={visible}
        // onOk={handleOk}
        onCancel={hideModal}
        width={'100vw'}
      >
        <Flex vertical gap={'middle'}>
          <Card title="geek">
            Public URL
            <Paragraph copyable>This is a copyable text.</Paragraph>
            <Space size={'middle'}>
              <Button>Preview</Button>
              <Button>Embedded</Button>
            </Space>
          </Card>
          <Card title="Backend service API">
            Service API Endpoint
            <Paragraph copyable>This is a copyable text.</Paragraph>
            <Space size={'middle'}>
              <Button onClick={showApiKeyModal}>Api Key</Button>
              <Button>Api Reference</Button>
            </Space>
          </Card>
          <Space>
            <b>Date Range:</b>
            <RangePicker
              disabledDate={disabledDate}
              value={pickerValue}
              onChange={setPickerValue}
            />
          </Space>
          <div className={styles.chartWrapper}>
            <LineChart></LineChart>
          </div>
        </Flex>
        <ChatApiKeyModal
          visible={apiKeyVisible}
          hideModal={hideApiKeyModal}
        ></ChatApiKeyModal>
      </Modal>
    </>
  );
};

export default ChatOverviewModal;
