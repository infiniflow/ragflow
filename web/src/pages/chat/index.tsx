import { ReactComponent as ChatAppCube } from '@/assets/svg/chat-app-cube.svg';
import { useSetModalState } from '@/hooks/commonHooks';
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
import classNames from 'classnames';
import { useCallback, useState } from 'react';
import ChatConfigurationModal from './chat-configuration-modal';
import ChatContainer from './chat-container';
import {
  useClickConversationCard,
  useClickDialogCard,
  useFetchConversationList,
  useFetchDialog,
  useGetChatSearchParams,
  useRemoveDialog,
  useSelectConversationList,
  useSelectFirstDialogOnMount,
  useSetCurrentDialog,
} from './hooks';

import styles from './index.less';

const Chat = () => {
  const dialogList = useSelectFirstDialogOnMount();
  const [activated, setActivated] = useState<string>('');
  const { visible, hideModal, showModal } = useSetModalState();
  const { setCurrentDialog, currentDialog } = useSetCurrentDialog();
  const { onRemoveDialog } = useRemoveDialog();
  const { handleClickDialog } = useClickDialogCard();
  const { handleClickConversation } = useClickConversationCard();
  const { dialogId, conversationId } = useGetChatSearchParams();
  const { list: conversationList, addTemporaryConversation } =
    useSelectConversationList();

  useFetchDialog(dialogId, true);

  const handleAppCardEnter = (id: string) => () => {
    setActivated(id);
  };

  const handleAppCardLeave = () => {
    setActivated('');
  };

  const handleShowChatConfigurationModal = (dialogId?: string) => () => {
    if (dialogId) {
      setCurrentDialog(dialogId);
    }
    showModal();
  };

  const handleDialogCardClick = (dialogId: string) => () => {
    handleClickDialog(dialogId);
  };

  const handleConversationCardClick = (dialogId: string) => () => {
    handleClickConversation(dialogId);
  };

  const handleCreateTemporaryConversation = useCallback(() => {
    addTemporaryConversation();
  }, [addTemporaryConversation]);

  const items: MenuProps['items'] = [
    {
      key: '1',
      onClick: handleCreateTemporaryConversation,
      label: (
        <Space>
          <EditOutlined /> New chat
        </Space>
      ),
    },
  ];

  const buildAppItems = (dialogId: string) => {
    const appItems: MenuProps['items'] = [
      {
        key: '1',
        onClick: handleShowChatConfigurationModal(dialogId),
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
        onClick: () => onRemoveDialog([dialogId]),
        label: (
          <Space>
            <DeleteOutlined />
            Delete chat
          </Space>
        ),
      },
    ];

    return appItems;
  };

  useFetchConversationList();

  return (
    <Flex className={styles.chatWrapper}>
      <Flex className={styles.chatAppWrapper}>
        <Flex flex={1} vertical>
          <Button type="primary" onClick={handleShowChatConfigurationModal()}>
            Create an Assistant
          </Button>
          <Divider></Divider>
          <Flex className={styles.chatAppContent} vertical gap={10}>
            {dialogList.map((x) => (
              <Card
                key={x.id}
                hoverable
                className={classNames(styles.chatAppCard, {
                  [styles.chatAppCardSelected]: dialogId === x.id,
                })}
                onMouseEnter={handleAppCardEnter(x.id)}
                onMouseLeave={handleAppCardLeave}
                onClick={handleDialogCardClick(x.id)}
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
                      <Dropdown menu={{ items: buildAppItems(x.id) }}>
                        <ChatAppCube className={styles.cubeIcon}></ChatAppCube>
                      </Dropdown>
                    </section>
                  )}
                </Flex>
              </Card>
            ))}
          </Flex>
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
          <Flex vertical gap={10} className={styles.chatTitleContent}>
            {conversationList.map((x) => (
              <Card
                key={x.id}
                hoverable
                onClick={handleConversationCardClick(x.id)}
                className={classNames(styles.chatTitleCard, {
                  [styles.chatTitleCardSelected]: x.id === conversationId,
                })}
              >
                <div>{x.name}</div>
              </Card>
            ))}
          </Flex>
        </Flex>
      </Flex>
      <Divider type={'vertical'} className={styles.divider}></Divider>
      <ChatContainer></ChatContainer>
      <ChatConfigurationModal
        visible={visible}
        showModal={showModal}
        hideModal={hideModal}
        id={currentDialog.id}
      ></ChatConfigurationModal>
    </Flex>
  );
};

export default Chat;
