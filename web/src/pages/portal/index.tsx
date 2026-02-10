import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useFetchAppConf } from '@/hooks/logic-hooks';
import { Loader2, MessageCircle, Search } from 'lucide-react';
import { useState } from 'react';
import {
  PublicDialog,
  generateShareUrl,
  useFetchPublicDialogs,
  useSearchKeywords,
} from './hooks';

export default function PortalPage() {
  const appConf = useFetchAppConf();
  const [currentPage, setCurrentPage] = useState(1);
  const [selectedDialog, setSelectedDialog] = useState<PublicDialog | null>(
    null,
  );
  const [isIframeLoading, setIsIframeLoading] = useState(false);
  const pageSize = 30;

  const { keywords, debouncedKeywords, handleSearch } = useSearchKeywords();
  const { data, isLoading, isError } = useFetchPublicDialogs(
    currentPage,
    pageSize,
    debouncedKeywords,
  );

  const handleDialogClick = (dialog: PublicDialog) => {
    setIsIframeLoading(true);
    setSelectedDialog(dialog);
  };

  const handleIframeLoad = () => {
    setIsIframeLoading(false);
  };

  const totalPages = data ? Math.ceil(data.total / pageSize) : 0;

  return (
    <div className="h-screen bg-muted/30 flex overflow-hidden relative p-4 lg:p-6 gap-4 lg:gap-6">
      {/* Left Sidebar - Card Style with rounded corners and border */}
      <div className="w-72 lg:w-80 flex flex-col bg-white dark:bg-gray-950 rounded-xl shadow-2xl border z-10 overflow-hidden flex-shrink-0">
        {/* Header - Compact */}
        <div className="px-4 py-4 border-b bg-white dark:bg-gray-950">
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

        {/* Search Bar */}
        <div className="px-3 py-3 bg-white dark:bg-gray-950">
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
        <div className="flex-1 overflow-y-auto bg-white dark:bg-gray-950">
          {isLoading && (
            <div className="flex justify-center items-center py-12">
              <Loader2 className="size-5 animate-spin text-primary" />
            </div>
          )}

          {isError && (
            <div className="px-3 py-8 text-center">
              <p className="text-destructive text-xs">加载失败</p>
            </div>
          )}

          {data && data.dialogs.length === 0 && (
            <div className="px-3 py-8 text-center">
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
                  className={`w-full px-3 py-2.5 flex items-center gap-2.5 hover:bg-muted/50 transition-colors text-left ${
                    selectedDialog?.id === dialog.id
                      ? 'bg-primary/10 border-l-2 border-primary'
                      : 'border-l-2 border-transparent'
                  }`}
                >
                  <RAGFlowAvatar
                    avatar={dialog.icon}
                    name={dialog.name}
                    className="size-8 flex-shrink-0"
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
                  <MessageCircle className="size-3.5 text-muted-foreground flex-shrink-0" />
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Pagination - Compact */}
        {data && totalPages > 1 && (
          <div className="px-3 py-2 border-t bg-white dark:bg-gray-950">
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

      {/* Right Panel - Chat Interface (Flex grow, right aligned) */}
      <div className="flex-1 flex flex-col bg-background min-w-0">
        {selectedDialog ? (
          /* Embedded Chat iframe - Full height with loading state */
          <div className="flex-1 overflow-hidden flex items-center justify-end relative rounded-lg">
            {/* Loading Overlay */}
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
                src={generateShareUrl(selectedDialog)}
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
