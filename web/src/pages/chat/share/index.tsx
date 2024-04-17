import { useEffect } from 'react';
import ChatContainer from '../chat-container';
import { useCreateConversationOnMount } from '../shared-hooks';

const SharedChat = () => {
  const x = useCreateConversationOnMount();

  useEffect(() => {
    console.info(location.href);
  }, []);

  return <ChatContainer></ChatContainer>;
};

export default SharedChat;
