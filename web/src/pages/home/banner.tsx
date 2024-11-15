import { Card, CardContent } from '@/components/ui/card';
import { ArrowRight } from 'lucide-react';

function BannerCard() {
  return (
    <Card className="w-auto border-none h-3/4">
      <CardContent className="p-4">
        <span className="inline-block bg-backgroundCoreWeak rounded-sm px-1 text-xs">
          System
        </span>
        <div className="flex mt-1 gap-4">
          <span className="text-lg truncate">Setting up your LLM</span>
          <ArrowRight />
        </div>
      </CardContent>
    </Card>
  );
}

function MyCard() {
  return (
    <div className="w-[265px] h-[87px] pl-[17px] pr-[13px] py-[15px] bg-[#b8b5cb]/20 rounded-xl border border-[#e6e3f6]/10 backdrop-blur-md justify-between items-end inline-flex">
      <div className="grow shrink basis-0 flex-col justify-start items-start gap-[9px] inline-flex">
        <div className="px-1 py-0.5 bg-[#644bf7] rounded justify-center items-center gap-2 inline-flex">
          <div className="text-white text-xs font-medium font-['IBM Plex Mono'] leading-none">
            System
          </div>
        </div>
        <div className="self-stretch text-white text-lg font-normal font-['Inter'] leading-7">
          Setting up your LLM
        </div>
      </div>
      <div className="w-6 h-6 relative" />
    </div>
  );
}

export function Banner() {
  return (
    <section className="bg-[url('@/assets/banner.png')] bg-cover h-28 rounded-2xl mx-14 my-8 flex gap-8 justify-between">
      <div className="h-full text-3xl font-bold items-center inline-flex ml-6">
        Welcome to RAGFlow
      </div>
      <div className="flex justify-between items-center gap-10 mr-5">
        <BannerCard></BannerCard>
        <BannerCard></BannerCard>
        <MyCard></MyCard>
      </div>
    </section>
  );
}
