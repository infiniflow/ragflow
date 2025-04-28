/**
 * Agent聊天页面
 * 该组件是Agent聊天界面的主要容器，包含三个主要部分：
 * 1. 左侧Agent列表：显示所有可用的Agent
 * 2. 中间对话列表：显示当前选中Agent的所有对话
 * 3. 右侧聊天窗口：显示当前选中对话的聊天内容
 *
 * 创建Agent支持权限设置，选择'团队'权限时将自动获取最新团队成员列表并设置共享权限
 */
import { useTheme } from '@/components/theme-provider';
import {
  useCreateAgent,
  useCreateConversation,
  useDeleteAgent,
  useFetchAgentList,
} from '@/hooks/agent-hooks';
import { useTranslate } from '@/hooks/common-hooks';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import {
  Button,
  Card,
  Divider,
  Empty,
  Flex,
  Input,
  Modal,
  Space,
  Spin,
  Typography,
  message,
} from 'antd';
import classNames from 'classnames';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import AgentChatContainer from './agent-chat-container';
import AgentSettingModal from './agent-setting-modal';
import AgentTemplateModal from './agent-template-modal';
import styles from './index.less';

const { Text } = Typography;

/**
 * Agent接口定义
 * 描述Agent的基本信息结构
 */
interface IAgent {
  id: string; // Agent唯一标识
  title: string; // Agent名称
  description?: string; // Agent描述（可选）
  avatar?: string; // Agent头像URL（可选）
}

/**
 * 对话接口定义
 * 描述Agent对话的基本信息结构
 */
interface IConversation {
  id: string; // 对话唯一标识
  title: string; // 对话标题
  agentId: string; // 对话所属的Agent ID
}

const AgentChat = () => {
  // 搜索字符串状态，用于过滤Agent列表
  const [searchString, setSearchString] = useState('');

  // 当前选中的Agent ID
  const [activeAgentId, setActiveAgentId] = useState<string | null>(null);

  // 当前选中的对话ID
  const [activeConversationId, setActiveConversationId] = useState<
    string | null
  >(null);

  // AbortController用于取消请求
  const [controller, setController] = useState(new AbortController());

  // 获取当前主题和翻译函数
  const { theme } = useTheme();
  const { t } = useTranslate('agent');

  /**
   * 创建Agent相关钩子
   * 提供模板选择、Agent设置等功能
   */
  const {
    templateModalVisible, // 模板选择对话框可见性
    hideTemplateModal, // 隐藏模板选择对话框
    showTemplateModal, // 显示模板选择对话框
    agentSettingVisible, // Agent设置对话框可见性
    hideAgentSettingModal, // 隐藏Agent设置对话框
    handleTemplateSelect, // 处理模板选择
    loading: createAgentLoading, // 创建Agent过程中的加载状态
    onAgentOk, // Agent创建确认回调
  } = useCreateAgent();

  /**
   * 获取Agent列表相关钩子
   * 提供Agent列表数据和加载状态
   */
  const {
    data: agentList, // Agent列表数据
    loading: fetchAgentLoading, // 获取Agent列表的加载状态
    refetch, // 重新获取Agent列表
  } = useFetchAgentList();

  /**
   * 删除Agent相关钩子
   * 提供删除Agent功能和加载状态
   */
  const {
    loading: deleteAgentLoading, // 删除Agent的加载状态
    deleteAgent, // 删除Agent函数
  } = useDeleteAgent();

  /**
   * 创建新对话相关钩子
   * 提供创建对话功能和加载状态
   */
  const { loading: createConversationLoading, createConversation } =
    useCreateConversation();

  /**
   * 模拟对话数据列表
   * 实际应用中应该从API获取
   */
  const [conversationList, setConversationList] = useState<IConversation[]>([]);

  /**
   * 处理搜索输入框变化
   * 更新搜索关键词状态
   */
  const handleInputChange: ChangeEventHandler<HTMLInputElement> = useCallback(
    (e) => {
      setSearchString(e.target.value);
    },
    [],
  );

  /**
   * 处理Agent卡片点击事件
   * 设置当前活动Agent并重置请求控制器
   */
  const handleAgentCardClick = useCallback(
    (agentId: string) => () => {
      setActiveAgentId(agentId);
      // 重置当前选中的对话
      setActiveConversationId(null);
      // 创建新的控制器以便取消之前的请求
      setController((pre) => {
        pre.abort();
        return new AbortController();
      });
      // 获取该Agent的对话列表
      // TODO: 这里应该请求API获取对话列表数据
    },
    [],
  );

  /**
   * 处理对话卡片点击事件
   * 设置当前活动对话
   */
  const handleConversationCardClick = useCallback(
    (conversationId: string) => () => {
      setActiveConversationId(conversationId);
    },
    [],
  );

  /**
   * 创建新对话
   * 使用当前选中的Agent创建一个新的对话
   */
  const handleCreateConversation = useCallback(async () => {
    if (!activeAgentId) {
      message.warning(t('selectAgentFirst'));
      return;
    }

    try {
      const result = await createConversation(
        activeAgentId,
        `新对话 ${conversationList.length + 1}`,
      );

      if (result) {
        // 创建成功后，添加到会话列表
        const newConversation: IConversation = {
          id: result.conversation_id || Date.now().toString(),
          title: `新对话 ${conversationList.length + 1}`,
          agentId: activeAgentId,
        };

        setConversationList((prev) => [...prev, newConversation]);

        // 选中新创建的会话
        setActiveConversationId(newConversation.id);
      }
    } catch (error) {
      console.error('创建新对话失败:', error);
      message.error(t('createConversationFailed'));
    }
  }, [activeAgentId, conversationList.length, createConversation, t]);

  /**
   * 确认删除Agent的对话框
   * 显示确认对话框并处理删除操作
   */
  const confirmDeleteAgent = useCallback(
    (agentId: string) => {
      Modal.confirm({
        title: t('confirmDelete'),
        content: t('confirmDeleteContent'),
        onOk: async () => {
          await deleteAgent(agentId);
        },
      });
    },
    [deleteAgent, t],
  );

  /**
   * 过滤Agent列表
   * 根据搜索关键词过滤Agent列表
   */
  const filteredAgentList = useMemo(() => {
    if (!searchString) return agentList;

    // 根据标题和描述进行过滤
    return agentList.filter(
      (agent: IAgent) =>
        agent.title.toLowerCase().includes(searchString.toLowerCase()) ||
        (agent.description &&
          agent.description.toLowerCase().includes(searchString.toLowerCase())),
    );
  }, [agentList, searchString]);

  /**
   * 组件挂载时获取Agent列表
   * 确保页面加载后展示最新数据
   */
  useEffect(() => {
    refetch().then(() => {
      console.log('获取Agent列表完成');
    });
  }, [refetch]);

  return (
    <Flex className={styles.agentChatWrapper}>
      {/* 左侧Agent列表区域 */}
      <Flex className={styles.agentAppWrapper}>
        <Flex flex={1} vertical>
          {/* 创建Agent按钮 */}
          <Button type="primary" onClick={showTemplateModal} block>
            {t('createAgent')}
          </Button>
          <Divider style={{ margin: '12px 0' }}></Divider>

          {/* Agent搜索框 */}
          <Input
            placeholder={t('searchAgent')}
            value={searchString}
            allowClear
            onChange={handleInputChange}
            prefix={<SearchOutlined />}
            style={{ marginBottom: '12px' }}
          />

          {/* Agent列表展示区域 */}
          <Flex className={styles.agentAppContent} vertical gap={10}>
            <Spin
              spinning={fetchAgentLoading}
              wrapperClassName={styles.agentSpin}
            >
              {filteredAgentList && filteredAgentList.length > 0 ? (
                filteredAgentList.map((agent: any) => (
                  <Card
                    key={agent.id}
                    hoverable
                    className={classNames(styles.agentAppCard, {
                      [theme === 'dark'
                        ? styles.agentAppCardSelectedDark
                        : styles.agentAppCardSelected]:
                        agent.id === activeAgentId,
                    })}
                    onClick={handleAgentCardClick(agent.id)}
                  >
                    <Flex align="center" justify="space-between">
                      <Flex align="center" style={{ overflow: 'hidden' }}>
                        {/* Agent头像 */}
                        <img
                          src={agent.avatar || '/logo.svg'}
                          alt=""
                          className={styles.agentCardIcon}
                          style={{ marginRight: '8px' }}
                        />
                        <Flex
                          vertical
                          style={{ width: 150, overflow: 'hidden' }}
                        >
                          {/* Agent标题 */}
                          <Text
                            className={styles.agentCardTitle}
                            ellipsis={{ tooltip: agent.title }}
                          >
                            {agent.title}
                          </Text>
                          {/* Agent描述 */}
                          <Text
                            type="secondary"
                            className={styles.agentCardDescription}
                            ellipsis={{ tooltip: agent.description }}
                          >
                            {agent.description || t('noDescription')}
                          </Text>
                        </Flex>
                      </Flex>
                      {/* Agent操作按钮 */}
                      <div
                        style={{ width: 30, textAlign: 'right', flexShrink: 0 }}
                      >
                        {/* 删除按钮 */}
                        <DeleteOutlined
                          className={styles.agentActionIcon}
                          onClick={(e) => {
                            e.stopPropagation();
                            confirmDeleteAgent(agent.id);
                          }}
                        />
                      </div>
                    </Flex>
                  </Card>
                ))
              ) : (
                <Empty description={t('noAgents')} />
              )}
            </Spin>
          </Flex>
        </Flex>
      </Flex>

      {/* 中间对话列表区域 */}
      <Flex className={styles.agentConversationWrapper}>
        <Flex flex={1} vertical>
          {/* 对话列表标题和新建对话按钮 */}
          <Flex justify="space-between" align="center">
            <span className={styles.agentConversationTitle}>
              {t('conversations')}
            </span>
            <div
              className={styles.addConversationBtn}
              onClick={handleCreateConversation}
            >
              <PlusOutlined />
            </div>
          </Flex>
          <Divider style={{ margin: '12px 0' }}></Divider>

          {/* 对话列表展示区域 */}
          <Flex className={styles.agentConversationContent} vertical gap={8}>
            {activeAgentId ? (
              conversationList
                .filter((conv) => conv.agentId === activeAgentId)
                .map((conversation) => (
                  <Card
                    key={conversation.id}
                    hoverable
                    className={classNames(styles.agentConversationCard, {
                      [styles.agentConversationCardSelected]:
                        conversation.id === activeConversationId,
                    })}
                    onClick={handleConversationCardClick(conversation.id)}
                  >
                    <Flex align="center" justify="space-between">
                      <span>{conversation.title}</span>
                      <Space>
                        <EditOutlined
                          className={styles.agentConversationIcon}
                        />
                        <DeleteOutlined
                          className={styles.agentConversationIcon}
                        />
                      </Space>
                    </Flex>
                  </Card>
                ))
            ) : (
              <Empty description={t('selectAgentFirst')} />
            )}
          </Flex>
        </Flex>
      </Flex>

      {/* 右侧聊天窗口 */}
      <AgentChatContainer
        controller={controller}
        conversationId={activeConversationId}
        agentId={activeAgentId}
      />

      {/* 模板选择模态框 */}
      {templateModalVisible && (
        <AgentTemplateModal
          visible={templateModalVisible}
          hideModal={hideTemplateModal}
          onTemplateSelect={handleTemplateSelect}
        />
      )}

      {/* Agent 设置模态框 */}
      {agentSettingVisible && (
        <AgentSettingModal
          visible={agentSettingVisible}
          hideModal={hideAgentSettingModal}
          onOk={onAgentOk}
          loading={createAgentLoading}
        />
      )}
    </Flex>
  );
};

export default AgentChat;
