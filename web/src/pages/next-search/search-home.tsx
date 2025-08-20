import { Input } from '@/components/originui/input';
import { Button } from '@/components/ui/button';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { cn } from '@/lib/utils';
import { Search } from 'lucide-react';
import { Dispatch, SetStateAction } from 'react';
import './index.less';
import Spotlight from './spotlight';

export default function SearchPage({
  isSearching,
  setIsSearching,
  searchText,
  setSearchText,
}: {
  isSearching: boolean;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
  searchText: string;
  setSearchText: Dispatch<SetStateAction<string>>;
}) {
  const { data: userInfo } = useFetchUserInfo();
  return (
    <section className="relative w-full flex transition-all justify-center items-center mt-32">
      <div className="relative z-10 px-8 pt-8 flex  text-transparent flex-col justify-center items-center w-[780px]">
        <h1
          className={cn(
            'text-4xl font-bold bg-gradient-to-r from-sky-600 from-30% via-sky-500 via-60% to-emerald-500 bg-clip-text',
          )}
        >
          RAGFlow
        </h1>

        <div className="rounded-lg  text-primary text-xl sticky flex justify-center w-full transform scale-100 mt-8 p-6 h-[230px] border">
          {!isSearching && <Spotlight className="z-0" />}
          <div className="flex flex-col justify-center items-center  w-2/3">
            {!isSearching && (
              <>
                <p className="mb-4 transition-opacity">👋 Hi there</p>
                <p className="mb-10 transition-opacity">
                  Welcome back, {userInfo?.nickname}
                </p>
              </>
            )}

            <div className="relative w-full ">
              <Input
                placeholder="How can I help you today?"
                className="w-full rounded-full py-6 px-4 pr-10 text-text-primary text-lg bg-bg-base delay-700"
                value={searchText}
                onKeyUp={(e) => {
                  if (e.key === 'Enter') {
                    setIsSearching(!isSearching);
                  }
                }}
                onChange={(e) => {
                  setSearchText(e.target.value || '');
                }}
              />
              <button
                type="button"
                className="absolute right-2 top-1/2 -translate-y-1/2 transform rounded-full bg-white p-2 text-gray-800 shadow w-12"
                onClick={() => {
                  setIsSearching(!isSearching);
                }}
              >
                <Search size={22} className="m-auto" />
              </button>
            </div>
          </div>
        </div>

        <div className="mt-14 w-full overflow-hidden opacity-100 max-h-96">
          <p className="text-text-primary mb-2 text-xl">Related Search</p>
          <div className="mt-2 flex flex-wrap justify-start gap-2">
            <Button
              variant="transparent"
              className="bg-bg-card text-text-secondary"
            >
              Related Search
            </Button>
            <Button
              variant="transparent"
              className="bg-bg-card text-text-secondary"
            >
              Related Search Related SearchRelated Search
            </Button>
            <Button
              variant="transparent"
              className="bg-bg-card text-text-secondary"
            >
              Related Search Search
            </Button>
            <Button
              variant="transparent"
              className="bg-bg-card text-text-secondary"
            >
              Related Search Related SearchRelated Search
            </Button>
            <Button
              variant="transparent"
              className="bg-bg-card text-text-secondary"
            >
              Related Search
            </Button>
          </div>
        </div>
      </div>
    </section>
  );
}
