import CopyToClipboard from '@/components/copy-to-clipboard';
import HightLightMarkdown from '@/components/highlight-markdown';
import { Domain } from '@/constants/common';
import { useTranslate } from '@/hooks/commonHooks';
import { IModalProps } from '@/interfaces/common';
import { Card, Modal, Tabs, TabsProps } from 'antd';
import styles from './index.less';

const EmbedModal = ({
  visible,
  hideModal,
  token = '',
}: IModalProps<any> & { token: string }) => {
  const { t } = useTranslate('chat');

  const text = `
  ~~~ html
  <iframe
  src="https://${Domain}/chat/share?shared_id=${token}"
  style="width: 100%; height: 100%; min-height: 600px"
  frameborder="0"
>
</iframe>
~~~
  `;

  const items: TabsProps['items'] = [
    {
      key: '1',
      label: t('fullScreenTitle'),
      children: (
        <Card
          title={t('fullScreenDescription')}
          extra={<CopyToClipboard text={text}></CopyToClipboard>}
          className={styles.codeCard}
        >
          <HightLightMarkdown>{text}</HightLightMarkdown>
        </Card>
      ),
    },
    {
      key: '2',
      label: t('partialTitle'),
      children: t('comingSoon'),
    },
    {
      key: '3',
      label: t('extensionTitle'),
      children: t('comingSoon'),
    },
  ];

  const onChange = (key: string) => {
    console.log(key);
  };

  return (
    <Modal
      title={t('embedModalTitle')}
      open={visible}
      style={{ top: 300 }}
      width={'50vw'}
      onOk={hideModal}
      onCancel={hideModal}
    >
      <Tabs defaultActiveKey="1" items={items} onChange={onChange} />
    </Modal>
  );
};

export default EmbedModal;
