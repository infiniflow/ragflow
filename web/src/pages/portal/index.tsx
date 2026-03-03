import MarkdownContent from '@/components/next-markdown-content';
import { ReferenceDocumentList } from '@/components/next-message-item/reference-document-list';
import { ReferenceImageList } from '@/components/next-message-item/reference-image-list';
import PdfSheet from '@/components/pdf-drawer';
import { useClickDrawer } from '@/components/pdf-drawer/hooks';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { MessageType } from '@/constants/chat';
import { useFetchAppConf } from '@/hooks/logic-hooks';
import {
  IReference,
  IReferenceObject,
  Message,
} from '@/interfaces/database/chat';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import relativeTime from 'dayjs/plugin/relativeTime';
import {
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Clock,
  Loader2,
  MessageSquare,
  Mic,
  Send,
  Trash2,
} from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';
import { DocumentSelector } from './document-selector';
import {
  PublicDialog,
  useFetchConversationHistory,
  useFetchPublicDialogs,
} from './hooks';

// 会话历史类型定义
interface ConversationHistory {
  id: string;
  dialog_id: string;
  dialog_name: string;
  dialog_icon?: string;
  title: string;
  last_message: string;
  message_count: number;
  create_time: number;
  update_time: number;
}

// 转换 IReference 到 IReferenceObject
const convertReferenceToObject = (
  reference: IReference | undefined,
): IReferenceObject | undefined => {
  if (!reference) return undefined;

  const chunks: Record<string, any> = {};
  const doc_aggs: Record<string, any> = {};

  reference.chunks?.forEach((chunk, index) => {
    chunks[index] = chunk;
  });

  reference.doc_aggs?.forEach((doc) => {
    doc_aggs[doc.doc_id] = doc;
  });

  return { chunks, doc_aggs };
};

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

export default function PortalPage() {
  const appConf = useFetchAppConf();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(true);
  const [selectedDialog, setSelectedDialog] = useState<PublicDialog | null>(
    null,
  );
  const [selectedConversationId, setSelectedConversationId] = useState<
    string | undefined
  >();
  const [messages, setMessages] = useState<Message[]>([]);
  const [inputValue, setInputValue] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [isChatMode, setIsChatMode] = useState(false); // 是否进入聊天模式
  const [welcomeMessage, setWelcomeMessage] = useState('');
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [conversationToDelete, setConversationToDelete] = useState<
    string | null
  >(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const loadingConversationIdRef = useRef<string | null>(null); // 跟踪正在加载的会话ID
  const [dialogPage, setDialogPage] = useState(1);
  const [allLoadedDialogs, setAllLoadedDialogs] = useState<PublicDialog[]>([]);
  const [selectedDocumentIds, setSelectedDocumentIds] = useState<string[]>([]);
  const [availableDocuments, setAvailableDocuments] = useState<
    Array<{ doc_id: string; doc_name: string }>
  >([]);
  const [messageReferences, setMessageReferences] = useState<
    Record<string, any>
  >({});

  // PDF 查看器
  const { visible, hideModal, documentId, selectedChunk, clickDocumentButton } =
    useClickDrawer();

  // 获取公开助手（支持分页）
  const { data: dialogsData, isLoading: dialogsLoading } =
    useFetchPublicDialogs(dialogPage, 9, '');

  // 累积加载的助手（去重）
  useEffect(() => {
    if (dialogsData?.dialogs) {
      if (dialogPage === 1) {
        setAllLoadedDialogs(dialogsData.dialogs);
      } else {
        setAllLoadedDialogs((prev) => {
          const existingIds = new Set(prev.map((d) => d.id));
          const newDialogs = dialogsData.dialogs.filter(
            (d) => !existingIds.has(d.id),
          );
          return [...prev, ...newDialogs];
        });
      }
    }
  }, [dialogsData, dialogPage]);

  const hasMoreDialogs = dialogsData
    ? allLoadedDialogs.length < dialogsData.total
    : false;

  // 获取会话历史
  const { data: conversationData, refetch: refetchConversations } =
    useFetchConversationHistory();
  const allConversations = conversationData?.conversations || [];

  // 自动滚动到底部
  useEffect(() => {
    if (isChatMode) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, isChatMode]);

  // 默认选中第一个助手并获取欢迎语
  useEffect(() => {
    if (!selectedDialog && allLoadedDialogs.length > 0) {
      fetchWelcomeMessage(allLoadedDialogs[0]);
    }
  }, [allLoadedDialogs.length]);

  const fetchWelcomeMessage = async (dialog: PublicDialog) => {
    setSelectedDialog(dialog);
    try {
      // 使用 dialog.id (真实的 dialog_id) 而不是 shared_id
      const response = await fetch(`/api/v1/chatbots/${dialog.id}/info`, {
        headers: {
          Authorization: `Bearer ${dialog.auth_token}`,
        },
      });
      const result = await response.json();

      if (result.code === 0 && result.data.prologue) {
        setWelcomeMessage(result.data.prologue);
      }

      // 获取关联的知识库文档
      if (
        result.code === 0 &&
        result.data.kb_ids &&
        result.data.kb_ids.length > 0
      ) {
        fetchAvailableDocuments(result.data.kb_ids);
      } else {
        setAvailableDocuments([]);
      }
    } catch (error) {
      console.error('Failed to fetch welcome message:', error);
    }
  };

  const fetchAvailableDocuments = async (kbIds: string[]) => {
    try {
      const allDocs: Array<{ doc_id: string; doc_name: string }> = [];

      for (const kbId of kbIds) {
        const response = await fetch(`/v1/document/list`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            kb_id: kbId,
            page: 1,
            page_size: 1000,
          }),
        });
        const result = await response.json();

        if (result.code === 0 && result.data.docs) {
          const docs = result.data.docs.map((doc: any) => ({
            doc_id: doc.id,
            doc_name: doc.name,
          }));
          allDocs.push(...docs);
        }
      }

      setAvailableDocuments(allDocs);
    } catch (error) {
      console.error('Failed to fetch documents:', error);
      setAvailableDocuments([]);
    }
  };

  const handleSelectDialog = async (dialog: PublicDialog) => {
    setSelectedDialog(dialog);
    setSelectedConversationId(undefined);
    setIsChatMode(false);
    setMessages([]);
    setSelectedDocumentIds([]);
    await fetchWelcomeMessage(dialog);
  };

  const handleSelectConversation = async (
    conversation: ConversationHistory,
  ) => {
    // 记录正在加载的会话ID
    loadingConversationIdRef.current = conversation.id;

    // 立即清空消息和设置加载状态，防止显示旧数据
    setMessages([]);
    setMessageReferences({});
    setIsLoading(true);
    setIsChatMode(true);

    let dialog = allLoadedDialogs.find((d) => d.id === conversation.dialog_id);

    // 如果在已加载的列表中找不到，尝试从后端获取
    if (!dialog) {
      console.log('Dialog not found in loaded list, fetching from backend...');
      try {
        const response = await fetch(`/v1/dialog/public/list`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            page: 1,
            page_size: 1000,
            orderby: 'create_time',
            desc: true,
            name: '',
          }),
        });
        const result = await response.json();
        if (result.code === 0 && result.data.dialogs) {
          dialog = result.data.dialogs.find(
            (d: PublicDialog) => d.id === conversation.dialog_id,
          );
        }
      } catch (error) {
        console.error('Failed to fetch dialog:', error);
        if (loadingConversationIdRef.current === conversation.id) {
          setIsLoading(false);
        }
        return;
      }
    }

    if (!dialog) {
      console.error('Dialog not found:', conversation.dialog_id);
      if (loadingConversationIdRef.current === conversation.id) {
        setIsLoading(false);
      }
      return;
    }

    // 设置选中的对话和会话ID
    setSelectedDialog(dialog);
    setSelectedConversationId(conversation.id);

    try {
      const response = await fetch(
        `/v1/dialog/public/conversation/${conversation.id}/messages`,
      );
      const result = await response.json();

      // 只有当前正在加载的会话ID还是这个时，才更新消息（防止快速切换导致的问题）
      if (loadingConversationIdRef.current === conversation.id) {
        if (result.code === 0 && result.data.messages) {
          setMessages(result.data.messages);

          // 加载引文数据
          if (result.data.reference && Array.isArray(result.data.reference)) {
            const refs: Record<string, any> = {};

            // 过滤出助手消息（排除第一条欢迎消息）
            const assistantMessages = result.data.messages.filter(
              (msg: any, index: number) =>
                msg.role === MessageType.Assistant && index > 0,
            );

            // 将引文数据与助手消息匹配
            assistantMessages.forEach((msg: any, assistantIndex: number) => {
              if (result.data.reference[assistantIndex]) {
                refs[msg.id] = result.data.reference[assistantIndex];
              }
            });

            setMessageReferences(refs);
          }
        }
      } else {
        console.log('Conversation changed, ignoring stale response');
      }
    } catch (error) {
      console.error('Failed to load conversation:', error);
    } finally {
      // 只有当前正在加载的会话ID还是这个时，才关闭加载状态
      if (loadingConversationIdRef.current === conversation.id) {
        setIsLoading(false);
      }
    }
  };

  const handleDeleteConversation = useCallback(
    async (conversationId: string) => {
      try {
        const response = await fetch(
          `/v1/dialog/public/conversation/${conversationId}`,
          {
            method: 'DELETE',
          },
        );
        const result = await response.json();

        if (result.code === 0) {
          // 删除成功，刷新会话列表
          refetchConversations();
          // 如果删除的是当前选中的会话，返回主页
          if (selectedConversationId === conversationId) {
            handleBackToHome();
          }
          // 关闭对话框
          setDeleteDialogOpen(false);
          setConversationToDelete(null);
        } else {
          console.error('Failed to delete conversation:', result.message);
        }
      } catch (error) {
        console.error('Error deleting conversation:', error);
      }
    },
    [selectedConversationId, refetchConversations],
  );

  const handleDeleteClick = useCallback((conversationId: string) => {
    setConversationToDelete(conversationId);
    setDeleteDialogOpen(true);
  }, []);

  const handleConfirmDelete = useCallback(() => {
    if (conversationToDelete) {
      handleDeleteConversation(conversationToDelete);
    }
  }, [conversationToDelete, handleDeleteConversation]);

  const handleBackToHome = () => {
    // 如果正在加载，先停止
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setIsChatMode(false);
    setMessages([]);
    setMessageReferences({});
    setSelectedConversationId(undefined);
    setInputValue('');
    setIsLoading(false);
    if (selectedDialog) {
      fetchWelcomeMessage(selectedDialog);
    }
  };

  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
      setIsLoading(false);
    }
  };

  const handleSendMessage = async () => {
    if (!inputValue.trim() || !selectedDialog || isLoading) return;

    // 进入聊天模式
    if (!isChatMode) {
      setIsChatMode(true);
    }

    const userMessage: Message = {
      role: MessageType.User,
      content: inputValue.trim(),
      id: `user-${Date.now()}`,
      doc_ids: selectedDocumentIds.length > 0 ? selectedDocumentIds : undefined,
    };

    setMessages((prev) => [...prev, userMessage]);
    const questionText = inputValue.trim();
    const docIds =
      selectedDocumentIds.length > 0 ? selectedDocumentIds : undefined;
    setInputValue('');
    setSelectedDocumentIds([]); // 清空选中的文档
    setIsLoading(true);

    // 创建 AbortController
    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    // 添加一个空的助手消息用于流式更新
    const assistantMessageId = `assistant-${Date.now()}`;
    const assistantMessageIndex = messages.length + 1; // 预先计算索引位置

    setMessages((prev) => [
      ...prev,
      {
        role: MessageType.Assistant,
        content: '',
        id: assistantMessageId,
      },
    ]);

    try {
      const response = await fetch(
        `/api/v1/chatbots/${selectedDialog.id}/completions`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            Authorization: `Bearer ${selectedDialog.auth_token}`,
          },
          body: JSON.stringify({
            question: questionText,
            session_id: selectedConversationId,
            quote: true,
            stream: true,
            doc_ids: docIds,
          }),
          signal: abortController.signal, // 添加 abort signal
        },
      );

      if (!response.ok || !response.body) {
        throw new Error('Failed to send message');
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          if (line.startsWith('data:')) {
            try {
              const jsonStr = line.slice(5).trim();
              if (!jsonStr) continue;

              const data = JSON.parse(jsonStr);

              if (data.code === 0 && data.data) {
                if (data.data === true) {
                  break;
                }

                if (data.data.answer) {
                  // 直接更新最后一条消息，避免遍历整个数组
                  setMessages((prev) => {
                    const newMessages = [...prev];
                    const lastMsg = newMessages[newMessages.length - 1];
                    if (lastMsg && lastMsg.id === assistantMessageId) {
                      lastMsg.content = data.data.answer;
                    }
                    return newMessages;
                  });
                }

                // 保存引文数据
                if (data.data.reference) {
                  setMessageReferences((prev) => ({
                    ...prev,
                    [assistantMessageId]: data.data.reference,
                  }));
                }

                if (data.data.session_id && !selectedConversationId) {
                  setSelectedConversationId(data.data.session_id);
                }
              }
            } catch (e) {
              console.error('Failed to parse SSE data:', e);
            }
          }
        }
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        console.log('Generation stopped by user');
      } else {
        console.error('Failed to send message:', error);
        setMessages((prev) =>
          prev.filter((msg) => msg.id !== assistantMessageId),
        );
      }
    } finally {
      abortControllerRef.current = null;
      setIsLoading(false);
      setTimeout(() => {
        refetchConversations();
      }, 1000);
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  return (
    <div className="h-screen bg-gradient-to-br from-blue-50 via-white to-purple-50 dark:from-gray-950 dark:via-gray-900 dark:to-gray-950 flex overflow-hidden">
      <TooltipProvider>
        {/* Left Sidebar - 历史记录 */}
        <div
          className={`${
            sidebarCollapsed ? 'w-12' : 'w-72'
          } transition-all duration-300 bg-white/80 dark:bg-gray-950/80 backdrop-blur-sm border-r flex flex-col flex-shrink-0`}
        >
          <div className="p-2 border-b flex items-center justify-between">
            {!sidebarCollapsed && (
              <div className="flex items-center gap-2">
                <Clock className="size-4 text-muted-foreground" />
                <span className="text-base font-medium">历史记录</span>
              </div>
            )}
            <Button
              size="sm"
              variant="ghost"
              className="h-8 w-8 p-0"
              onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
            >
              {sidebarCollapsed ? (
                <ChevronRight className="size-4" />
              ) : (
                <ChevronLeft className="size-4" />
              )}
            </Button>
          </div>

          {!sidebarCollapsed && (
            <div className="flex-1 overflow-y-auto">
              {allConversations.length === 0 ? (
                <div className="px-4 py-8 text-center">
                  <MessageSquare className="size-10 text-muted-foreground/30 mx-auto mb-2" />
                  <p className="text-sm text-muted-foreground">暂无历史会话</p>
                </div>
              ) : (
                <div className="py-1">
                  {allConversations.map((conversation) => (
                    <div
                      key={conversation.id}
                      className={`group relative px-3 py-3 hover:bg-muted/50 transition-colors cursor-pointer ${
                        selectedConversationId === conversation.id
                          ? 'bg-primary/10 border-l-2 border-primary'
                          : 'border-l-2 border-transparent'
                      }`}
                      onClick={() => handleSelectConversation(conversation)}
                    >
                      <div className="flex items-start gap-2">
                        <MessageSquare className="size-4 text-muted-foreground flex-shrink-0 mt-1" />
                        <div className="flex-1 min-w-0">
                          <div
                            className="font-medium text-base truncate mb-1"
                            title={conversation.title}
                          >
                            {conversation.title}
                          </div>
                          <div className="text-sm text-muted-foreground/70 truncate mb-1">
                            {conversation.dialog_name}
                          </div>
                          {conversation.last_message && (
                            <div className="text-sm text-muted-foreground/60 truncate mb-1.5">
                              {conversation.last_message}
                            </div>
                          )}
                          <div className="flex items-center gap-1.5 text-sm text-muted-foreground">
                            <span>
                              {dayjs(conversation.update_time).fromNow()}
                            </span>
                            <span>·</span>
                            <span>{conversation.message_count} 条</span>
                          </div>
                        </div>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-6 w-6 p-0 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                          onClick={(e) => {
                            e.stopPropagation();
                            handleDeleteClick(conversation.id);
                          }}
                        >
                          <Trash2 className="size-3.5 text-destructive" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Main Content Area */}
        <div className="flex-1 flex flex-col min-w-0">
          {isChatMode ? (
            // 聊天模式
            <>
              {/* Chat Header */}
              <div className="h-14 border-b bg-white/80 dark:bg-gray-950/80 backdrop-blur-sm flex items-center justify-between px-6 flex-shrink-0">
                {selectedDialog && (
                  <div className="flex items-center gap-2">
                    <RAGFlowAvatar
                      avatar={selectedDialog.icon}
                      name={selectedDialog.name}
                      className="size-6"
                    />
                    <span className="text-base font-medium">
                      {selectedDialog.name}
                    </span>
                  </div>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleBackToHome}
                  className="text-blue-600 hover:text-blue-700 hover:bg-blue-50 h-9 px-3 text-base"
                >
                  <ArrowLeft className="size-4 mr-1.5" />
                  返回
                </Button>
              </div>

              {/* Messages Area */}
              <div className="flex-1 overflow-y-auto px-6 py-6">
                {isLoading && messages.length === 0 ? (
                  // 加载中的过渡效果
                  <div className="flex items-center justify-center h-full">
                    <div className="text-center">
                      <Loader2 className="size-12 animate-spin text-blue-500 mx-auto mb-4" />
                      <p className="text-base text-muted-foreground">
                        加载历史消息中...
                      </p>
                    </div>
                  </div>
                ) : (
                  <div className="w-full space-y-8">
                    {messages.map((msg, i) => (
                      <div
                        key={msg.id}
                        className={`flex gap-4 ${
                          msg.role === MessageType.User
                            ? 'justify-end'
                            : 'justify-start'
                        }`}
                      >
                        {msg.role === MessageType.Assistant &&
                          selectedDialog && (
                            <RAGFlowAvatar
                              avatar={selectedDialog.icon}
                              name={selectedDialog.name}
                              className="size-9 flex-shrink-0"
                            />
                          )}
                        <div
                          className={`px-5 py-3 rounded-2xl ${
                            msg.role === MessageType.User
                              ? 'bg-blue-500 text-white shadow-lg max-w-[50%]'
                              : 'bg-white dark:bg-gray-800 shadow-sm border border-gray-200 max-w-[70%]'
                          }`}
                        >
                          {msg.role === MessageType.User ? (
                            <p className="text-base leading-relaxed whitespace-pre-wrap break-words">
                              {msg.content}
                            </p>
                          ) : (
                            <div className="space-y-3">
                              <MarkdownContent
                                content={msg.content}
                                loading={isLoading && i === messages.length - 1}
                                reference={convertReferenceToObject(
                                  messageReferences[msg.id],
                                )}
                                clickDocumentButton={clickDocumentButton}
                              />
                              {/* 只有当消息有引文数据时才显示引文组件 */}
                              {messageReferences[msg.id] && (
                                <>
                                  <ReferenceImageList
                                    referenceChunks={
                                      messageReferences[msg.id]?.chunks
                                    }
                                    messageContent={msg.content}
                                  />
                                  {messageReferences[msg.id]?.doc_aggs?.length >
                                    0 && (
                                    <ReferenceDocumentList
                                      list={messageReferences[msg.id].doc_aggs}
                                    />
                                  )}
                                </>
                              )}
                            </div>
                          )}
                        </div>
                        {msg.role === MessageType.User && (
                          <div className="size-9 flex-shrink-0" />
                        )}
                      </div>
                    ))}
                    <div ref={messagesEndRef} />
                  </div>
                )}
              </div>

              {/* Chat Input */}
              <div className="border-t bg-white/80 dark:bg-gray-950/80 backdrop-blur-sm p-6 flex-shrink-0">
                <div className="max-w-4xl mx-auto">
                  <div className="flex gap-3 items-end">
                    <div className="flex-1 space-y-2">
                      {availableDocuments.length > 0 && (
                        <DocumentSelector
                          documents={availableDocuments}
                          selectedDocumentIds={selectedDocumentIds}
                          onSelectionChange={setSelectedDocumentIds}
                          disabled={isLoading}
                        />
                      )}
                      <div className="relative">
                        <Input
                          value={inputValue}
                          onChange={(e) => setInputValue(e.target.value)}
                          onKeyPress={handleKeyPress}
                          placeholder="输入消息..."
                          disabled={isLoading}
                          className="pr-20 h-12 text-lg rounded-xl shadow-sm !border-2 !border-blue-200 focus-visible:!border-blue-400 focus-visible:!ring-2 focus-visible:!ring-blue-200"
                        />
                        <div className="absolute right-2 top-1/2 -translate-y-1/2 flex gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-8 w-8 p-0"
                            disabled={isLoading}
                          >
                            <Mic className="size-4" />
                          </Button>
                          {isLoading ? (
                            <Button
                              size="sm"
                              onClick={handleStopGeneration}
                              variant="destructive"
                              className="h-8 px-3"
                            >
                              停止
                            </Button>
                          ) : (
                            <Button
                              size="sm"
                              onClick={handleSendMessage}
                              disabled={!inputValue.trim()}
                              className="h-8 w-8 p-0"
                            >
                              <Send className="size-4" />
                            </Button>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </>
          ) : (
            // 主页模式
            <div className="flex-1 flex flex-col items-center justify-center px-6 py-12">
              <div className="w-full max-w-4xl mx-auto space-y-20">
                {/* Logo and Welcome */}
                <div className="text-center space-y-6">
                  <div className="flex items-center justify-center gap-3 mb-8">
                    <img
                      src="/logo.gif"
                      alt="logo"
                      className="w-14 h-14 object-contain"
                    />
                    <h1 className="text-5xl font-bold text-blue-600">
                      {appConf.appName}
                    </h1>
                  </div>
                  <p className="text-xl text-muted-foreground font-medium">
                    👋 {welcomeMessage || '有什么我能帮你分担的吗？'}
                  </p>
                </div>

                {/* Large Input Box */}
                <div className="w-full">
                  <div className="space-y-3">
                    {availableDocuments.length > 0 && (
                      <div className="flex justify-center">
                        <DocumentSelector
                          documents={availableDocuments}
                          selectedDocumentIds={selectedDocumentIds}
                          onSelectionChange={setSelectedDocumentIds}
                          disabled={!selectedDialog || isLoading}
                        />
                      </div>
                    )}
                    <div className="relative">
                      <Input
                        value={inputValue}
                        onChange={(e) => setInputValue(e.target.value)}
                        onKeyPress={handleKeyPress}
                        placeholder="在此输入问题"
                        disabled={!selectedDialog || isLoading}
                        className="w-full h-20 text-xl px-6 pr-28 rounded-2xl shadow-xl border-2 border-blue-200 focus:border-blue-400 focus:ring-2 focus:ring-blue-200 transition-all"
                      />
                      <div className="absolute right-3 top-1/2 -translate-y-1/2 flex gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          className="h-12 w-12 p-0 rounded-full hover:bg-blue-50"
                        >
                          <Mic className="size-5 text-blue-600" />
                        </Button>
                        <Button
                          size="sm"
                          onClick={handleSendMessage}
                          disabled={
                            !selectedDialog || !inputValue.trim() || isLoading
                          }
                          className="h-12 w-12 p-0 rounded-full bg-gradient-to-r from-blue-500 to-cyan-500 hover:from-blue-600 hover:to-cyan-600"
                        >
                          {isLoading ? (
                            <Loader2 className="size-5 animate-spin" />
                          ) : (
                            <Send className="size-5" />
                          )}
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Assistant Cards - 瀑布流 */}
                <div className="w-full pt-8">
                  {dialogsLoading && dialogPage === 1 ? (
                    <div className="flex justify-center py-12">
                      <Loader2 className="size-8 animate-spin text-primary" />
                    </div>
                  ) : (
                    <>
                      <div className="grid grid-cols-3 gap-6">
                        {allLoadedDialogs.map((dialog) => (
                          <button
                            key={dialog.id}
                            onClick={() => handleSelectDialog(dialog)}
                            className={`group p-6 rounded-2xl border-2 transition-all text-left hover:shadow-xl hover:-translate-y-1 ${
                              selectedDialog?.id === dialog.id
                                ? 'border-blue-400 bg-blue-50 shadow-lg ring-2 ring-blue-200'
                                : 'border-gray-200 bg-white dark:bg-gray-800 hover:border-gray-300 hover:shadow-md'
                            }`}
                          >
                            <div className="flex items-center gap-3 mb-3">
                              <RAGFlowAvatar
                                avatar={dialog.icon}
                                name={dialog.name}
                                className="size-12"
                              />
                              <span
                                className={`font-semibold text-lg truncate flex-1 ${
                                  selectedDialog?.id === dialog.id
                                    ? 'text-blue-700'
                                    : ''
                                }`}
                              >
                                {dialog.name}
                              </span>
                            </div>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <p className="text-base text-muted-foreground line-clamp-2 leading-relaxed">
                                  {dialog.description || '暂无描述'}
                                </p>
                              </TooltipTrigger>
                              {dialog.description && (
                                <TooltipContent className="max-w-sm">
                                  <p className="text-base">
                                    {dialog.description}
                                  </p>
                                </TooltipContent>
                              )}
                            </Tooltip>
                          </button>
                        ))}
                      </div>

                      {/* 加载更多按钮 */}
                      {hasMoreDialogs && (
                        <div className="flex justify-center mt-8">
                          <Button
                            onClick={() => setDialogPage((prev) => prev + 1)}
                            disabled={dialogsLoading}
                            variant="outline"
                            className="px-8 py-2 text-blue-600 border-blue-200 hover:bg-blue-50 hover:border-blue-400"
                          >
                            {dialogsLoading ? (
                              <>
                                <Loader2 className="size-4 mr-2 animate-spin" />
                                加载中...
                              </>
                            ) : (
                              '加载更多助手'
                            )}
                          </Button>
                        </div>
                      )}
                    </>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>
      </TooltipProvider>

      {/* 删除确认对话框 */}
      <AlertDialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除这个会话吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setConversationToDelete(null)}>
              取消
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmDelete}
              className="bg-destructive hover:bg-destructive/90"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* PDF 查看器 */}
      {visible && (
        <PdfSheet
          visible={visible}
          hideModal={hideModal}
          documentId={documentId}
          chunk={selectedChunk}
        />
      )}
    </div>
  );
}
