import { Input } from '@/components/originui/input';
import { cn } from '@/lib/utils';
import { Search, X } from 'lucide-react';
import { Dispatch, SetStateAction } from 'react';
import './index.less';

export default function SearchingPage({
  isSearching,
  setIsSearching,
}: {
  isSearching: boolean;
  setIsSearching: Dispatch<SetStateAction<boolean>>;
}) {
  return (
    <section
      className={cn(
        'relative w-full flex transition-all justify-start items-center',
      )}
    >
      <div
        className={cn(
          'relative z-10 px-8 pt-8 flex  text-transparent justify-start items-start w-full',
        )}
      >
        <h1
          className={cn(
            'text-4xl font-bold bg-gradient-to-r from-sky-600 from-30% via-sky-500 via-60% to-emerald-500 bg-clip-text',
          )}
        >
          RAGFlow
        </h1>

        <div
          className={cn(
            ' rounded-lg text-primary text-xl sticky flex justify-center w-2/3 max-w-[780px] transform scale-100 ml-16 ',
          )}
        >
          <div className={cn('flex flex-col justify-start items-start w-full')}>
            <div className="relative w-full text-primary">
              <Input
                placeholder="How can I help you today?"
                className={cn(
                  'w-full rounded-full py-6 pl-4 !pr-[8rem] text-primary text-lg bg-background',
                )}
              />
              <div className="absolute right-2 top-1/2 -translate-y-1/2 transform flex items-center gap-1">
                <X />|
                <button
                  type="button"
                  className="rounded-full bg-white p-1 text-gray-800 shadow w-12 h-8 ml-4"
                  onClick={() => {
                    setIsSearching(!isSearching);
                  }}
                >
                  <Search size={22} className="m-auto" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}
