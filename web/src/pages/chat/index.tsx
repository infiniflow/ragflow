import { FormOutlined } from '@ant-design/icons';
import { Button, Card, Divider, Flex, Space, Tag } from 'antd';
import { useSelector } from 'umi';
import ChatContainer from './chat-container';

import ModalManager from '@/components/modal-manager';
import ChatConfigurationModal from './chat-configuration-modal';
import styles from './index.less';

const Chat = () => {
  const { name } = useSelector((state: any) => state.chatModel);

  return (
    <Flex className={styles.chatWrapper}>
      <Flex className={styles.chatAppWrapper}>
        <Flex flex={1} vertical>
          <ModalManager>
            {({ visible, showModal, hideModal }) => {
              return (
                <>
                  <Button type="primary" onClick={() => showModal()}>
                    Create an Assistant
                  </Button>
                  <ChatConfigurationModal
                    visible={visible}
                    showModal={showModal}
                    hideModal={hideModal}
                  ></ChatConfigurationModal>
                </>
              );
            }}
          </ModalManager>

          <Divider></Divider>
          <Card>
            <p>Card content</p>
          </Card>
        </Flex>
      </Flex>
      <Divider type={'vertical'} className={styles.divider}></Divider>
      <Flex className={styles.chatTitleWrapper}>
        <Flex flex={1} vertical>
          <Flex
            justify={'space-between'}
            align="center"
            className={styles.chatTitle}
          >
            <Space>
              <b>Chat</b>
              <Tag>25</Tag>
            </Space>
            <FormOutlined />
          </Flex>
          <Divider></Divider>
          <section className={styles.chatTitleContent}>today</section>
        </Flex>
      </Flex>
      <Divider type={'vertical'} className={styles.divider}></Divider>
      <ChatContainer></ChatContainer>
    </Flex>
  );
};

export default Chat;
