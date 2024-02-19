import { Button, Flex, Input } from 'antd';
import { ChangeEventHandler, useState } from 'react';

import styles from './index.less';

const ChatContainer = () => {
  const [value, setValue] = useState('');

  const handlePressEnter = () => {
    console.info(value);
  };

  const handleInputChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    setValue(e.target.value);
  };

  return (
    <Flex flex={1} className={styles.chatContainer} vertical>
      <Flex flex={1}>xx</Flex>
      <Input
        size="large"
        placeholder="Message Resume Assistant..."
        value={value}
        suffix={
          <Button type="primary" onClick={handlePressEnter}>
            Send
          </Button>
        }
        onPressEnter={handlePressEnter}
        onChange={handleInputChange}
      />
    </Flex>
  );
};

export default ChatContainer;
