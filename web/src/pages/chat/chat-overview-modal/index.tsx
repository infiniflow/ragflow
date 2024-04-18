import CopyToClipboard from '@/components/copy-to-clipboard';
import LineChart from '@/components/line-chart';
import { useCreatePublicUrlToken } from '@/hooks/chatHooks';
import { useSetModalState, useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { IDialog, IStats } from '@/interfaces/database/chat';
import { ReloadOutlined } from '@ant-design/icons';
import { Button, Card, DatePicker, Flex, Modal, Space, Typography } from 'antd';
import { RangePickerProps } from 'antd/es/date-picker';
import dayjs from 'dayjs';
import camelCase from 'lodash/camelCase';
import { Link } from 'umi';
import ChatApiKeyModal from '../chat-api-key-modal';
import { useFetchStatsOnMount, useSelectChartStatsList } from '../hooks';
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
  const { urlWithToken, createUrlToken, token } = useCreatePublicUrlToken(
    dialog.id,
    visible,
  );

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
        onCancel={hideModal}
        width={'100vw'}
      >
        <Flex vertical gap={'middle'}>
          <Card title={dialog.name}>
            <Flex gap={8} vertical>
              {t('publicUrl')}
              <Flex className={styles.linkText} gap={10}>
                <span>{urlWithToken}</span>
                <CopyToClipboard text={urlWithToken}></CopyToClipboard>
                <ReloadOutlined onClick={createUrlToken} />
              </Flex>
              <Space size={'middle'}>
                <Button>
                  <Link to={`/chat/share?shared_id=${token}`} target="_blank">
                    {t('preview')}
                  </Link>
                </Button>
                <Button>{t('embedded')}</Button>
              </Space>
            </Flex>
          </Card>
          <Card title={t('backendServiceApi')}>
            <Flex gap={8} vertical>
              {t('serviceApiEndpoint')}
              <Paragraph copyable className={styles.linkText}>
                This is a copyable text.
              </Paragraph>
            </Flex>
            <Space size={'middle'}>
              <Button onClick={showApiKeyModal}>{t('apiKey')}</Button>
              <Button>{t('apiReference')}</Button>
            </Space>
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
      </Modal>
    </>
  );
};

export default ChatOverviewModal;
