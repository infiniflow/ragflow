import LineChart from '@/components/line-chart';
import { useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { Button, Card, DatePicker, Modal, Space, Typography } from 'antd';

import styles from './index.less';

const { Paragraph } = Typography;
const { RangePicker } = DatePicker;

const ChatOverviewModal = ({ visible, hideModal }: IModalProps<any>) => {
  const { t } = useTranslate('chat');

  return (
    <>
      <Modal
        title={t('overview')}
        open={visible}
        // onOk={handleOk}
        onCancel={hideModal}
        width={700}
      >
        <Card title="Backend service API">
          Service API Endpoint
          <Paragraph copyable>This is a copyable text.</Paragraph>
          <Space size={'middle'}>
            <Button>Api Key</Button>
            <Button>Api Reference</Button>
          </Space>
        </Card>
        <RangePicker />
        <div className={styles.chartWrapper}>
          <LineChart></LineChart>
        </div>
      </Modal>
    </>
  );
};

export default ChatOverviewModal;
