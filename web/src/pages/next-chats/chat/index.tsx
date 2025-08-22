import EmbedDialog from '@/components/embed-dialog';
import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchConversation,
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { ArrowUpRight, LogOut, Send } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { ChatSettings } from './app-settings/chat-settings';
import { MultipleChatBox } from './chat-box/multiple-chat-box';
import { SingleChatBox } from './chat-box/single-chat-box';
import { Sessions } from './sessions';
import { useAddChatBox } from './use-add-box';
import { useSwitchDebugMode } from './use-switch-debug-mode';

export default function Chat() {
  const { id } = useParams();
  const { navigateToChatList } = useNavigatePage();
  const { data } = useFetchDialog();
  const { t } = useTranslation();
  const { data: conversation } = useFetchConversation();

  const { handleConversationCardClick, controller } =
    useHandleClickConversationCard();
  const { visible: settingVisible, switchVisible: switchSettingVisible } =
    useSetModalState(true);
  const {
    removeChatBox,
    addChatBox,
    chatBoxIds,
    hasSingleChatBox,
    hasThreeChatBox,
  } = useAddChatBox();

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  const { conversationId, isNew } = useGetChatSearchParams();

  const { isDebugMode, switchDebugMode } = useSwitchDebugMode();

  if (isDebugMode) {
    return (
      <section className="pt-14 h-[100vh] pb-24">
        <div className="flex items-center justify-between px-10 pb-5">
          <span className="text-2xl">
            Multiple Models ({chatBoxIds.length}/3)
          </span>
          <Button variant={'ghost'} onClick={switchDebugMode}>
            Exit <LogOut />
          </Button>
        </div>
        <MultipleChatBox
          chatBoxIds={chatBoxIds}
          controller={controller}
          removeChatBox={removeChatBox}
          addChatBox={addChatBox}
        ></MultipleChatBox>
      </section>
    );
  }

  return (
    <section className="h-full flex flex-col pr-5">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToChatList}>
                {t('chat.chat')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{data.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
        <Button onClick={showEmbedModal}>
          <Send />
          {t('common.embedIntoSite')}
        </Button>
      </PageHeader>
      <div className="flex flex-1 min-h-0 pb-9">
        <Sessions
          hasSingleChatBox={hasSingleChatBox}
          handleConversationCardClick={handleConversationCardClick}
          switchSettingVisible={switchSettingVisible}
        ></Sessions>

        <Card className="flex-1 min-w-0 bg-transparent border h-full">
          <CardContent className="flex p-0 h-full">
            <Card className="flex flex-col flex-1 bg-transparent min-w-0">
              <CardHeader
                className={cn('p-5', { 'border-b': hasSingleChatBox })}
              >
                <CardTitle className="flex justify-between items-center text-base">
                  <div className="truncate">{conversation.name}</div>
                  <Button
                    variant={'ghost'}
                    onClick={switchDebugMode}
                    disabled={
                      hasThreeChatBox ||
                      isEmpty(conversationId) ||
                      isNew === 'true'
                    }
                  >
                    <ArrowUpRight /> Multiple Models
                  </Button>
                </CardTitle>
              </CardHeader>
              <CardContent className="flex-1 p-0 min-h-0">
                <SingleChatBox controller={controller}></SingleChatBox>
              </CardContent>
            </Card>
            {settingVisible && (
              <ChatSettings
                switchSettingVisible={switchSettingVisible}
              ></ChatSettings>
            )}
          </CardContent>
        </Card>
      </div>
      {embedVisible && (
        <EmbedDialog
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          from={SharedFrom.Chat}
          beta={beta}
          isAgent={false}
        ></EmbedDialog>
      )}
    </section>
  );
}
