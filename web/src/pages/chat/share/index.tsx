import { useEffect } from 'react';
import {
  useCreateSharedConversationOnMount,
  useSelectCurrentSharedConversation,
  useSendSharedMessage,
} from '../shared-hooks';
import ChatContainer from './large';

import styles from './index.less';

const SharedChat = () => {
  const { conversationId } = useCreateSharedConversationOnMount();
  const {
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    ref,
    loading,
    setCurrentConversation,
  } = useSelectCurrentSharedConversation(conversationId);

  const {
    handlePressEnter,
    handleInputChange,
    value,
    loading: sendLoading,
  } = useSendSharedMessage(
    currentConversation,
    addNewestConversation,
    removeLatestMessage,
    setCurrentConversation,
  );

  useEffect(() => {
    console.info(location.href);
  }, []);

  return (
    <div className={styles.chatWrapper}>
      <ChatContainer
        value={value}
        handleInputChange={handleInputChange}
        handlePressEnter={handlePressEnter}
        loading={loading}
        sendLoading={sendLoading}
        conversation={currentConversation}
        ref={ref}
      ></ChatContainer>
    </div>
  );
};

export default SharedChat;
