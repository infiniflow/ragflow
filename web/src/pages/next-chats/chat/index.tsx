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
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import {
  useFetchConversationList,
  useFetchConversationManually,
  useFetchDialog,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import { useMount } from 'ahooks';
import { isEmpty } from 'lodash';
import { ArrowUpRight, LogOut, Send } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
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
  const [currentConversation, setCurrentConversation] =
    useState<IClientConversation>({} as IClientConversation);

  const { fetchConversationManually } = useFetchConversationManually();

  const { handleConversationCardClick, controller, stopOutputMessage } =
    useHandleClickConversationCard();

  const { isDebugMode, switchDebugMode } = useSwitchDebugMode();
  const { removeChatBox, addChatBox, chatBoxIds, hasSingleChatBox } =
    useAddChatBox(isDebugMode);

  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  const { conversationId, isNew } = useGetChatSearchParams();

  const { data: dialogList } = useFetchConversationList();

  const currentConversationName = useMemo(() => {
    return dialogList.find((x) => x.id === conversationId)?.name;
  }, [conversationId, dialogList]);

  const fetchConversation: typeof handleConversationCardClick = useCallback(
    async (conversationId, isNew) => {
      if (conversationId && !isNew) {
        const conversation = await fetchConversationManually(conversationId);
        if (!isEmpty(conversation)) {
          setCurrentConversation(conversation);
        }
      }
    },
    [fetchConversationManually],
  );

  const handleSessionClick: typeof handleConversationCardClick = useCallback(
    (conversationId, isNew) => {
      handleConversationCardClick(conversationId, isNew);
      fetchConversation(conversationId, isNew);
    },
    [fetchConversation, handleConversationCardClick],
  );

  useMount(() => {
    fetchConversation(conversationId, isNew === 'true');
  });

  if (isDebugMode) {
    return (
      <section className="pt-14 h-[100vh] pb-24">
        <div className="flex items-center justify-between px-10 pb-5">
          <span className="text-2xl">
            {t('chat.multipleModels')} ({chatBoxIds.length}/3)
          </span>
          <Button variant={'ghost'} onClick={switchDebugMode}>
            {t('chat.exit')} <LogOut />
          </Button>
        </div>
        <MultipleChatBox
          chatBoxIds={chatBoxIds}
          controller={controller}
          removeChatBox={removeChatBox}
          addChatBox={addChatBox}
          stopOutputMessage={stopOutputMessage}
          conversation={currentConversation}
        ></MultipleChatBox>
      </section>
    );
  }

  return (
    <section className="h-full flex flex-col">
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
        <Sessions handleConversationCardClick={handleSessionClick}></Sessions>

        <Card className="flex-1 min-w-0 bg-transparent border-none shadow-none h-full">
          <CardContent className="flex p-0 h-full">
            <Card className="flex flex-col flex-1 bg-transparent min-w-0">
              <CardHeader
                className={cn('p-5', { 'border-b': hasSingleChatBox })}
              >
                <CardTitle className="flex justify-between items-center text-base">
                  <div className="truncate">{currentConversationName}</div>
                  <Button variant={'ghost'} onClick={switchDebugMode}>
                    <ArrowUpRight /> {t('chat.multipleModels')}
                  </Button>
                </CardTitle>
              </CardHeader>
              <CardContent className="flex-1 p-0 min-h-0">
                <SingleChatBox
                  controller={controller}
                  stopOutputMessage={stopOutputMessage}
                  conversation={currentConversation}
                ></SingleChatBox>
              </CardContent>
            </Card>
            <ChatSettings hasSingleChatBox={hasSingleChatBox}></ChatSettings>
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
