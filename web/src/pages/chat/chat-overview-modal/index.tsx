import LineChart from '@/components/line-chart';
import { useSetModalState, useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { IDialog, IStats } from '@/interfaces/database/chat';
import { Button, Card, DatePicker, Flex, Modal, Space, Typography } from 'antd';
import { RangePickerProps } from 'antd/es/date-picker';
import dayjs from 'dayjs';
import camelCase from 'lodash/camelCase';
import ChatApiKeyModal from '../chat-api-key-modal';
import EmbedModal from '../embed-modal';
import {
  useFetchStatsOnMount,
  usePreviewChat,
  useSelectChartStatsList,
  useShowEmbedModal,
} from '../hooks';
import styles from './index.less';

const { Paragraph } = Typography;
const { RangePicker } = DatePicker;

const ChatOverviewModal = ({
  visible,
  hideModal,
  dialog,
}: IModalProps<any> & { dialog: IDialog }) => {
  const { t } = useTranslate('chat');
  const chartList = useSelectChartStatsList();
  const {
    visible: apiKeyVisible,
    hideModal: hideApiKeyModal,
    showModal: showApiKeyModal,
  } = useSetModalState();
  const {
    embedVisible,
    hideEmbedModal,
    showEmbedModal,
    embedToken,
    errorContextHolder,
  } = useShowEmbedModal(dialog.id);

  const { pickerValue, setPickerValue } = useFetchStatsOnMount(visible);

  const disabledDate: RangePickerProps['disabledDate'] = (current) => {
    return current && current > dayjs().endOf('day');
  };

  const { handlePreview, contextHolder } = usePreviewChat(dialog.id);

  return (
    <>
      <Modal
        title={t('overview')}
        open={visible}
        onCancel={hideModal}
        width={'100vw'}
      >
        <Flex vertical gap={'middle'}>
          <Card title={t('backendServiceApi')}>
            <Flex gap={8} vertical>
              {t('serviceApiEndpoint')}
              <Paragraph copyable className={styles.linkText}>
                https://demo.ragflow.io/v1/api/
              </Paragraph>
            </Flex>
            <Space size={'middle'}>
              <Button onClick={showApiKeyModal}>{t('apiKey')}</Button>
              <a
                href={
                  'https://github.com/infiniflow/ragflow/blob/main/docs/conversation_api.md'
                }
                target="_blank"
                rel="noreferrer"
              >
                <Button>{t('apiReference')}</Button>
              </a>
            </Space>
          </Card>
          <Card title={dialog.name}>
            <Flex gap={8} vertical>
              {t('publicUrl')}
              {/* <Flex className={styles.linkText} gap={10}>
                <span>{urlWithToken}</span>
                <CopyToClipboard text={urlWithToken}></CopyToClipboard>
                <ReloadOutlined onClick={createUrlToken} />
              </Flex> */}
              <Space size={'middle'}>
                <Button onClick={handlePreview}>{t('preview')}</Button>
                <Button onClick={showEmbedModal}>{t('embedded')}</Button>
              </Space>
            </Flex>
          </Card>

          <Space>
            <b>{t('dateRange')}</b>
            <RangePicker
              disabledDate={disabledDate}
              value={pickerValue}
              onChange={setPickerValue}
              allowClear={false}
            />
          </Space>
          <div className={styles.chartWrapper}>
            {Object.keys(chartList).map((x) => (
              <div key={x} className={styles.chartItem}>
                <b className={styles.chartLabel}>{t(camelCase(x))}</b>
                <LineChart data={chartList[x as keyof IStats]}></LineChart>
              </div>
            ))}
          </div>
        </Flex>
        <ChatApiKeyModal
          visible={apiKeyVisible}
          hideModal={hideApiKeyModal}
          dialogId={dialog.id}
        ></ChatApiKeyModal>
        <EmbedModal
          token={embedToken}
          visible={embedVisible}
          hideModal={hideEmbedModal}
        ></EmbedModal>
        {contextHolder}
        {errorContextHolder}
      </Modal>
    </>
  );
};

export default ChatOverviewModal;
