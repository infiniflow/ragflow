import FloatingChatWidget from '@/components/floating-chat-widget';

const ChatWidget = () => {
  return (
    <div
      style={{
        background: 'transparent',
        margin: 0,
        padding: 0,
      }}
    >
      <style>{`
        html, body { 
          background: transparent !important; 
          margin: 0; 
          padding: 0; 
        }
        #root {
          background: transparent !important;
        }
      `}</style>
      <FloatingChatWidget />
    </div>
  );
};

export default ChatWidget;
