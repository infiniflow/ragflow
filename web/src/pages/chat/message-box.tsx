// RCE CSS
import { MessageList } from 'react-chat-elements';
import 'react-chat-elements/dist/main.css';

const ChatBox = () => {
  return (
    <div style={{ width: 600 }}>
      {/* <MessageBox
        position={'left'}
        type={'photo'}
        text={'react.svg'}
        data={{
          uri: 'https://facebook.github.io/react/img/logo.svg',
          status: {
            click: false,
            loading: 0,
          },
        }}
      /> */}

      <MessageList
        // referance={messageListReferance}
        className="message-list"
        lockable={true}
        toBottomHeight={'100%'}
        dataSource={[
          {
            position: 'right',
            type: 'text',
            text: 'Lorem ipsum dolor sit amet',
            date: new Date(),
          },
          {
            position: 'left',
            type: 'text',
            text: 'Lorem ipsum dolor sit amet',
            date: new Date(),
          },
        ]}
      />
    </div>
  );
};

export default ChatBox;
