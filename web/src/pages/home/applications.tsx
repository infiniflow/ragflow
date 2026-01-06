import { CardSineLineContainer } from '@/components/card-singleline-container';
import { EmptyCardType } from '@/components/empty/constant';
import { EmptyAppCard } from '@/components/empty/empty';
import { HomeIcon } from '@/components/svg-icon';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { Routes } from '@/routes';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Agents } from './agent-list';
import { SeeAllAppCard } from './application-card';
import { ChatList } from './chat-list';
import { MemoryList } from './memory-list';
import { SearchList } from './search-list';

const IconMap = {
  [Routes.Chats]: 'chats',
  [Routes.Searches]: 'searches',
  [Routes.Agents]: 'agents',
  [Routes.Memories]: 'memory',
};

const EmptyTypeMap = {
  [Routes.Chats]: EmptyCardType.Chat,
  [Routes.Searches]: EmptyCardType.Search,
  [Routes.Agents]: EmptyCardType.Agent,
  [Routes.Memories]: EmptyCardType.Memory,
};

export function Applications() {
  const [val, setVal] = useState(Routes.Chats);
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [listLength, setListLength] = useState(0);
  const [loading, setLoading] = useState(false);

  const handleNavigate = useCallback(
    ({ isCreate }: { isCreate?: boolean }) => {
      if (isCreate) {
        navigate(val + '?isCreate=true');
      } else {
        navigate(val);
      }
    },
    [navigate, val],
  );

  const options = useMemo(
    () => [
      { value: Routes.Chats, label: t('chat.chatApps') },
      { value: Routes.Searches, label: t('search.searchApps') },
      { value: Routes.Agents, label: t('header.flow') },
      { value: Routes.Memories, label: t('memories.memory') },
    ],
    [t],
  );

  const handleChange = (path: SegmentedValue) => {
    setVal(path as Routes);
    setListLength(0);
    setLoading(true);
  };

  return (
    <section className="mt-12">
      <div className="flex justify-between items-center mb-5">
        <h2 className="text-2xl font-semibold flex gap-2.5">
          <HomeIcon
            name={`${IconMap[val as keyof typeof IconMap]}`}
            width={'32'}
          />
          {options.find((x) => x.value === val)?.label}
        </h2>
        <Segmented
          options={options}
          value={val}
          onChange={handleChange}
          buttonSize="xl"
          // className="bg-bg-card border border-border-button rounded-lg"
          // activeClassName="bg-text-primary border-none rounded-lg"
        ></Segmented>
      </div>
      {/* <div className="flex flex-wrap gap-4"> */}
      <CardSineLineContainer>
        {val === Routes.Agents && (
          <Agents
            setListLength={(length: number) => setListLength(length)}
            setLoading={(loading: boolean) => setLoading(loading)}
          ></Agents>
        )}
        {val === Routes.Chats && (
          <ChatList
            setListLength={(length: number) => setListLength(length)}
            setLoading={(loading: boolean) => setLoading(loading)}
          ></ChatList>
        )}
        {val === Routes.Searches && (
          <SearchList
            setListLength={(length: number) => setListLength(length)}
            setLoading={(loading: boolean) => setLoading(loading)}
          ></SearchList>
        )}
        {val === Routes.Memories && (
          <MemoryList
            setListLength={(length: number) => setListLength(length)}
            setLoading={(loading: boolean) => setLoading(loading)}
          ></MemoryList>
        )}
        {listLength > 0 && (
          <SeeAllAppCard
            click={() => handleNavigate({ isCreate: false })}
          ></SeeAllAppCard>
        )}
      </CardSineLineContainer>
      {listLength <= 0 && !loading && (
        <EmptyAppCard
          type={EmptyTypeMap[val as keyof typeof EmptyTypeMap]}
          onClick={() => handleNavigate({ isCreate: true })}
        />
      )}
      {/* </div> */}
    </section>
  );
}
