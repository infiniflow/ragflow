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
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchDialog } from '@/hooks/use-chat-request';
import { cn } from '@/lib/utils';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { ChatSettings } from './app-settings/chat-settings';
import { MultipleChatBox } from './chat-box/multiple-chat-box';
import { SingleChatBox } from './chat-box/single-chat-box';
import { Sessions } from './sessions';
import { useAddChatBox } from './use-add-box';

export default function Chat() {
  const { navigateToChatList } = useNavigatePage();
  const { data } = useFetchDialog();
  const { t } = useTranslation();
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
      </PageHeader>
      <div className="flex flex-1 min-h-0">
        <Sessions
          handleConversationCardClick={handleConversationCardClick}
          switchSettingVisible={switchSettingVisible}
        ></Sessions>

        <Card className="flex-1 min-w-0 bg-transparent border h-full">
          <CardContent className="flex p-0 h-full">
            <Card className="flex flex-col flex-1 bg-transparent">
              <CardHeader
                className={cn('p-5', { 'border-b': hasSingleChatBox })}
              >
                <CardTitle className="flex justify-between items-center">
                  <div className="text-base">
                    Card Title
                    <Button variant={'ghost'} className="ml-2">
                      GPT-4
                    </Button>
                  </div>
                  <Button
                    variant={'ghost'}
                    onClick={addChatBox}
                    disabled={hasThreeChatBox}
                  >
                    <Plus></Plus> Multiple Models
                  </Button>
                </CardTitle>
              </CardHeader>
              <CardContent className="flex-1 p-0">
                {hasSingleChatBox ? (
                  <SingleChatBox controller={controller}></SingleChatBox>
                ) : (
                  <MultipleChatBox
                    chatBoxIds={chatBoxIds}
                    controller={controller}
                    removeChatBox={removeChatBox}
                  ></MultipleChatBox>
                )}
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
    </section>
  );
}
