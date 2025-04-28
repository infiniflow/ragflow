import MessageInput from '@/components/message-input';
import { useTranslate } from '@/hooks/common-hooks';
import { Flex, Spin } from 'antd';
import { ChangeEventHandler, memo, useRef, useState } from 'react';
import styles from './index.less';

interface IProps {
  controller: AbortController;
  conversationId: string | null;
  agentId: string | null;
}

const AgentChatContainer = ({
  controller,
  conversationId,
  agentId,
}: IProps) => {
  const { t } = useTranslate('agent');
  const [value, setValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [sendLoading, setSendLoading] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // 模拟消息数据
  const messages = [];

  // 处理输入变化
  const handleInputChange: ChangeEventHandler<HTMLTextAreaElement> = (e) => {
    setValue(e.target.value);
  };

  // 处理发送消息
  const handlePressEnter = () => {
    if (!value.trim()) return;
    if (!conversationId || !agentId) {
      console.warn('未选择对话或Agent');
      return;
    }
    // 这里仅示例UI，实际逻辑后续实现
    console.log(
      '发送消息:',
      value,
      '对话ID:',
      conversationId,
      'AgentID:',
      agentId,
    );
    setValue('');
  };

  // 停止生成
  const stopOutputMessage = () => {
    controller.abort();
    setSendLoading(false);
  };

  return (
    <Flex flex={1} className={styles.agentChatContainer} vertical>
      <Flex flex={1} vertical className={styles.messageContainer}>
        <div>
          <Spin spinning={loading}>
            <div className={styles.messagePlaceholder}>
              {!conversationId ? (
                <div className={styles.emptyMessage}>
                  {t('selectOrCreateConversation')}
                </div>
              ) : messages.length === 0 ? (
                <div className={styles.emptyMessage}>{t('noMessages')}</div>
              ) : (
                <div className={styles.messageList}>
                  {/* 消息列表组件将在这里 */}
                </div>
              )}
            </div>
          </Spin>
        </div>
        <div ref={ref} />
      </Flex>

      <MessageInput
        disabled={!conversationId || !agentId}
        sendDisabled={!value.trim() || !conversationId || !agentId}
        sendLoading={sendLoading}
        value={value}
        onInputChange={handleInputChange}
        onPressEnter={handlePressEnter}
        conversationId={conversationId || ''}
        createConversationBeforeUploadDocument={() => Promise.resolve('')}
        stopOutputMessage={stopOutputMessage}
      />
    </Flex>
  );
};

export default memo(AgentChatContainer);
