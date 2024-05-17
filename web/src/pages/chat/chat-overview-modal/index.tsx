import LineChart from '@/components/line-chart';
import { Domain } from '@/constants/common';
import { useSetModalState, useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { IDialog, IStats } from '@/interfaces/database/chat';
import { formatDate } from '@/utils/date';
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

const StatsLineChart = ({ statsType }: { statsType: keyof IStats }) => {
  const { t } = useTranslate('chat');
  const chartList = useSelectChartStatsList();
  const list =
    chartList[statsType]?.map((x) => ({
      ...x,
      xAxis: formatDate(x.xAxis),
    })) ?? [];

  return (
    <div className={styles.chartItem}>
      <b className={styles.chartLabel}>{t(camelCase(statsType))}</b>
      <LineChart data={list}></LineChart>
    </div>
  );
};

const ChatOverviewModal = ({
  visible,
  hideModal,
  dialog,
}: IModalProps<any> & { dialog: IDialog }) => {
  const { t } = useTranslate('chat');
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
        cancelButtonProps={{ style: { display: 'none' } }}
        onOk={hideModal}
        width={'100vw'}
        okText={t('close', { keyPrefix: 'common' })}
      >
        <Flex vertical gap={'middle'}>
          <Card title={t('backendServiceApi')}>
            <Flex gap={8} vertical>
              {t('serviceApiEndpoint')}
              <Paragraph copyable className={styles.linkText}>
                https://
                {location.hostname === Domain ? Domain : '<YOUR_MACHINE_IP>'}
                /v1/api/
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
          <Card title={`${dialog.name} Web App`}>
            <Flex gap={8} vertical>
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
            <StatsLineChart statsType={'pv'}></StatsLineChart>
            <StatsLineChart statsType={'round'}></StatsLineChart>
            <StatsLineChart statsType={'speed'}></StatsLineChart>
            <StatsLineChart statsType={'thumb_up'}></StatsLineChart>
            <StatsLineChart statsType={'tokens'}></StatsLineChart>
            <StatsLineChart statsType={'uv'}></StatsLineChart>
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
