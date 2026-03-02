import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useFetchAppConf } from '@/hooks/logic-hooks';
import dayjs from 'dayjs';
import 'dayjs/locale/zh-cn';
import relativeTime from 'dayjs/plugin/relativeTime';
import {
  Clock,
  Loader2,
  MessageCircle,
  MessageSquare,
  Search,
  Trash2,
} from 'lucide-react';
import { useCallback, useState } from 'react';
import {
  PublicDialog,
  generateShareUrl,
  useFetchConversationHistory,
  useFetchPublicDialogs,
  useSearchKeywords,
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

dayjs.extend(relativeTime);
dayjs.locale('zh-cn');

export default function PortalPage() {
  const appConf = useFetchAppConf();
  const [currentPage, setCurrentPage] = useState(1);
  const [selectedDialog, setSelectedDialog] = useState<PublicDialog | null>(
    null,
  );
  const [isIframeLoading, setIsIframeLoading] = useState(false);
  const [selectedConversationId, setSelectedConversationId] = useState<
    string | undefined
  >();
  const [iframeKey, setIframeKey] = useState(0);
  const pageSize = 30;

  const { keywords, debouncedKeywords, handleSearch } = useSearchKeywords();
  const { data, isLoading, isError } = useFetchPublicDialogs(
    currentPage,
    pageSize,
    debouncedKeywords,
  );

  // 从后端查询所有公开助手的会话历史
  const { data: conversationData, refetch: refetchConversations } =
    useFetchConversationHistory();
  const allConversations = conversationData?.conversations || [];

  const handleDialogClick = (dialog: PublicDialog) => {
    setIsIframeLoading(true);
    setSelectedDialog(dialog);
    setSelectedConversationId(undefined);
    setIframeKey((prev) => prev + 1);
  };

  const handleSelectConversation = (conversation: ConversationHistory) => {
    // 找到对应的助手
    const dialog = data?.dialogs.find((d) => d.id === conversation.dialog_id);
    if (dialog) {
      setIsIframeLoading(true);
      setSelectedDialog(dialog);
      setSelectedConversationId(conversation.id);
      setIframeKey((prev) => prev + 1);
    }
  };

  const handleDeleteConversation = useCallback(
    (conversationId: string) => {
      // TODO: 调用后端API删除会话
      // 目前只是刷新列表
      refetchConversations();
      if (selectedConversationId === conversationId) {
        setSelectedConversationId(undefined);
        setIframeKey((prev) => prev + 1);
      }
    },
    [selectedConversationId, refetchConversations],
  );

  const handleNewConversation = () => {
    if (selectedDialog) {
      setSelectedConversationId(undefined);
      setIframeKey((prev) => prev + 1);
      setIsIframeLoading(true);
      // 延迟刷新会话列表，等待新会话创建
      setTimeout(() => {
        refetchConversations();
      }, 2000);
    }
  };

  const handleIframeLoad = () => {
    setIsIframeLoading(false);
  };

  const totalPages = data ? Math.ceil(data.total / pageSize) : 0;

  return (
    <div className="h-screen bg-muted/30 flex overflow-hidden relative p-4 lg:p-6 gap-4 lg:gap-6">
      {/* Left Sidebar - 上下布局 */}
      <div className="w-72 lg:w-80 flex flex-col bg-white dark:bg-gray-950 rounded-xl shadow-2xl border overflow-hidden flex-shrink-0">
        {/* Header */}
        <div className="px-4 py-4 border-b bg-white dark:bg-gray-950 flex-shrink-0">
          <div className="flex items-center gap-2 mb-3">
            <img
              src="/logo.gif"
              alt="logo"
              className="w-7 h-7 object-contain"
            />
            <span className="font-semibold text-base truncate">
              {appConf.appName}
            </span>
          </div>
          <h2 className="text-sm font-medium text-muted-foreground">
            AI 助手中心
          </h2>
        </div>

        {/* 上半部分：助手列表 */}
        <div className="flex-1 flex flex-col min-h-0 border-b">
          <div className="px-3 py-2 bg-muted/30 flex-shrink-0">
            <h3 className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
              <MessageCircle className="size-3.5" />
              助手列表
            </h3>
          </div>

          {/* Search Bar */}
          <div className="px-3 py-2 bg-white dark:bg-gray-950 flex-shrink-0">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 transform -translate-y-1/2 text-muted-foreground size-3.5" />
              <Input
                type="text"
                placeholder="搜索助手..."
                className="pl-8 h-8 text-xs bg-muted/50"
                value={keywords}
                onChange={(e) => handleSearch(e.target.value)}
              />
            </div>
          </div>

          {/* Assistant List */}
          <div className="flex-1 overflow-y-auto bg-white dark:bg-gray-950 min-h-0">
            {isLoading && (
              <div className="flex justify-center items-center py-8">
                <Loader2 className="size-5 animate-spin text-primary" />
              </div>
            )}

            {isError && (
              <div className="px-3 py-6 text-center">
                <p className="text-destructive text-xs">加载失败</p>
              </div>
            )}

            {data && data.dialogs.length === 0 && (
              <div className="px-3 py-6 text-center">
                <p className="text-muted-foreground text-xs">
                  {debouncedKeywords ? '无匹配结果' : '暂无助手'}
                </p>
              </div>
            )}

            {data && data.dialogs.length > 0 && (
              <div className="py-1">
                {data.dialogs.map((dialog) => (
                  <button
                    key={dialog.id}
                    onClick={() => handleDialogClick(dialog)}
                    className={`w-full px-3 py-2 flex items-center gap-2.5 hover:bg-muted/50 transition-colors text-left ${
                      selectedDialog?.id === dialog.id
                        ? 'bg-primary/10 border-l-2 border-primary'
                        : 'border-l-2 border-transparent'
                    }`}
                  >
                    <RAGFlowAvatar
                      avatar={dialog.icon}
                      name={dialog.name}
                      className="size-7 flex-shrink-0"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-xs truncate">
                        {dialog.name}
                      </div>
                      <div
                        className="text-[10px] text-muted-foreground truncate"
                        title={dialog.description || '暂无描述'}
                      >
                        {dialog.description || '暂无描述'}
                      </div>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Pagination */}
          {data && totalPages > 1 && (
            <div className="px-3 py-2 border-t bg-white dark:bg-gray-950 flex-shrink-0">
              <div className="flex items-center justify-between text-[10px] text-muted-foreground mb-1.5">
                <span>
                  {currentPage}/{totalPages}
                </span>
                <span>共 {data.total} 个</span>
              </div>
              <div className="flex gap-1.5">
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 h-7 text-[10px]"
                  disabled={currentPage === 1}
                  onClick={() => setCurrentPage((p) => Math.max(1, p - 1))}
                >
                  上一页
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1 h-7 text-[10px]"
                  disabled={currentPage === totalPages}
                  onClick={() =>
                    setCurrentPage((p) => Math.min(totalPages, p + 1))
                  }
                >
                  下一页
                </Button>
              </div>
            </div>
          )}
        </div>

        {/* 下半部分：历史会话 */}
        <div className="flex-1 flex flex-col min-h-0">
          <div className="px-3 py-2 bg-muted/30 flex-shrink-0 flex items-center justify-between">
            <h3 className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
              <Clock className="size-3.5" />
              历史会话
            </h3>
            {selectedDialog && (
              <Button
                size="sm"
                variant="ghost"
                className="h-6 px-2 text-[10px]"
                onClick={handleNewConversation}
              >
                新对话
              </Button>
            )}
          </div>

          {/* History List */}
          <div className="flex-1 overflow-y-auto bg-white dark:bg-gray-950 min-h-0">
            {allConversations.length === 0 ? (
              <div className="px-4 py-8 text-center">
                <MessageSquare className="size-10 text-muted-foreground/30 mx-auto mb-2" />
                <p className="text-xs text-muted-foreground">暂无历史会话</p>
              </div>
            ) : (
              <div className="py-1">
                {allConversations.map((conversation) => (
                  <div
                    key={conversation.id}
                    className={`group relative px-3 py-2 hover:bg-muted/50 transition-colors cursor-pointer ${
                      selectedConversationId === conversation.id
                        ? 'bg-primary/10 border-l-2 border-primary'
                        : 'border-l-2 border-transparent'
                    }`}
                    onClick={() => handleSelectConversation(conversation)}
                  >
                    <div className="flex items-start gap-2">
                      <MessageSquare className="size-3 text-muted-foreground flex-shrink-0 mt-0.5" />
                      <div className="flex-1 min-w-0">
                        <div
                          className="font-medium text-xs truncate mb-0.5"
                          title={conversation.title}
                        >
                          {conversation.title}
                        </div>
                        <div className="text-[10px] text-muted-foreground/70 truncate mb-0.5">
                          {conversation.dialog_name}
                        </div>
                        <div className="flex items-center gap-1.5 text-[10px] text-muted-foreground">
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
                        className="h-5 w-5 p-0 opacity-0 group-hover:opacity-100 transition-opacity flex-shrink-0"
                        onClick={(e) => {
                          e.stopPropagation();
                          handleDeleteConversation(conversation.id);
                        }}
                      >
                        <Trash2 className="size-3 text-destructive" />
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* History Count */}
          {allConversations.length > 0 && (
            <div className="px-3 py-1.5 border-t bg-white dark:bg-gray-950 flex-shrink-0">
              <p className="text-[10px] text-muted-foreground text-center">
                共 {allConversations.length} 条历史记录
              </p>
            </div>
          )}
        </div>
      </div>

      {/* Right Panel - Chat Interface */}
      <div className="flex-1 flex flex-col bg-background min-w-0">
        {selectedDialog ? (
          <div className="flex-1 overflow-hidden flex items-center justify-end relative rounded-lg">
            {isIframeLoading && (
              <div className="absolute inset-0 flex items-center justify-center bg-background/80 backdrop-blur-sm z-20 rounded-lg">
                <div className="text-center">
                  <Loader2 className="size-12 animate-spin text-primary mx-auto mb-4" />
                  <p className="text-sm text-muted-foreground">
                    正在加载对话...
                  </p>
                </div>
              </div>
            )}

            <div className="w-full h-full">
              <iframe
                key={iframeKey}
                src={
                  selectedConversationId
                    ? `${generateShareUrl(selectedDialog)}&conversation_id=${selectedConversationId}`
                    : generateShareUrl(selectedDialog)
                }
                className="w-full h-full border-0 rounded-lg shadow-lg bg-white"
                title={`Chat with ${selectedDialog.name}`}
                onLoad={handleIframeLoad}
              />
            </div>
          </div>
        ) : (
          <div className="flex-1 flex items-center justify-center rounded-lg">
            <div className="text-center max-w-sm">
              <div className="text-5xl mb-4">💬</div>
              <h3 className="text-lg font-medium mb-2">
                请选择一个助手开始对话
              </h3>
              <p className="text-sm text-muted-foreground">
                从左侧列表中选择助手
              </p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
