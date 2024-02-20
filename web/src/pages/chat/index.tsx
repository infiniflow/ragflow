import { DeleteOutlined, EditOutlined, FormOutlined } from '@ant-design/icons';
import {
  Button,
  Card,
  Divider,
  Dropdown,
  Flex,
  MenuProps,
  Space,
  Tag,
} from 'antd';
import ChatContainer from './chat-container';

import { ReactComponent as ChatAppCube } from '@/assets/svg/chat-app-cube.svg';
import ModalManager from '@/components/modal-manager';
import classNames from 'classnames';
import ChatConfigurationModal from './chat-configuration-modal';
import { useFetchDialogList } from './hooks';

import { useState } from 'react';
import styles from './index.less';

const Chat = () => {
  const dialogList = useFetchDialogList();
  const [activated, setActivated] = useState<string>('');

  const handleAppCardEnter = (id: string) => () => {
    setActivated(id);
  };

  const handleAppCardLeave = () => {
    setActivated('');
  };

  const items: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <a
          target="_blank"
          rel="noopener noreferrer"
          href="https://www.antgroup.com"
        >
          1st menu item
        </a>
      ),
    },
  ];

  const appItems: MenuProps['items'] = [
    {
      key: '1',
      label: (
        <Space>
          <EditOutlined />
          Edit
        </Space>
      ),
    },
    { type: 'divider' },
    {
      key: '2',
      label: (
        <Space>
          <DeleteOutlined />
          Delete chat
        </Space>
      ),
    },
  ];

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
          <Space direction={'vertical'} size={'middle'}>
            {dialogList.map((x) => (
              <Card
                key={x.id}
                className={classNames(styles.chatAppCard)}
                onMouseEnter={handleAppCardEnter(x.id)}
                onMouseLeave={handleAppCardLeave}
              >
                <Flex justify="space-between" align="center">
                  <Space>
                    {x.icon}
                    <section>
                      <b>{x.name}</b>
                      <div>{x.description}</div>
                    </section>
                  </Space>
                  {activated === x.id && (
                    <section>
                      <Dropdown menu={{ items: appItems }}>
                        <ChatAppCube className={styles.cubeIcon}></ChatAppCube>
                      </Dropdown>
                    </section>
                  )}
                </Flex>
              </Card>
            ))}
          </Space>
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
            <Dropdown menu={{ items }}>
              <FormOutlined />
            </Dropdown>
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
