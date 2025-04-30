import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { Routes } from '@/routes';
import { Cpu, MessageSquare, Search } from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';
import { Agents } from './agent-list';
import { ApplicationCard, SeeAllAppCard } from './application-card';
import { ChatList } from './chat-list';

const applications = [
  {
    id: 1,
    title: 'Jarvis chatbot',
    type: 'Chat app',
    update_time: '11/24/2024',
    avatar: <MessageSquare className="h-6 w-6" />,
  },
  {
    id: 2,
    title: 'Search app 01',
    type: 'Search app',
    update_time: '11/24/2024',
    avatar: <Search className="h-6 w-6" />,
  },
  {
    id: 3,
    title: 'Chatbot 01',
    type: 'Chat app',
    update_time: '11/24/2024',
    avatar: <MessageSquare className="h-6 w-6" />,
  },
  {
    id: 4,
    title: 'Workflow 01',
    type: 'Agent',
    update_time: '11/24/2024',
    avatar: <Cpu className="h-6 w-6" />,
  },
];

const All = 'all';

export function Applications() {
  const [val, setVal] = useState('all');
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleNavigate = useCallback(() => {
    navigate(val);
  }, [navigate, val]);

  const options = useMemo(
    () => [
      {
        label: 'All',
        value: All,
      },
      { value: Routes.Chats, label: t('header.chat') },
      { value: Routes.Searches, label: t('header.search') },
      { value: Routes.Agents, label: t('header.flow') },
    ],
    [t],
  );

  const handleChange = (path: SegmentedValue) => {
    setVal(path as string);
  };

  return (
    <section className="mt-12">
      <div className="flex justify-between items-center mb-5">
        <h2 className="text-2xl font-bold ">Applications</h2>
        <Segmented
          options={options}
          value={val}
          onChange={handleChange}
          className="bg-transparent"
        ></Segmented>
      </div>
      <div className="flex flex-wrap gap-4">
        {(val === All || val === Routes.Searches) &&
          [...Array(12)].map((_, i) => {
            const app = applications[i % 4];
            return <ApplicationCard key={i} app={app}></ApplicationCard>;
          })}
        {val === Routes.Agents && <Agents></Agents>}
        {val === Routes.Chats && <ChatList></ChatList>}
        {val === All || <SeeAllAppCard click={handleNavigate}></SeeAllAppCard>}
      </div>
    </section>
  );
}
