import { Card, CardContent } from '@/components/ui/card';
import { ArrowRight, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

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

export function Banner() {
  return (
    <section className="bg-[url('@/assets/banner.png')] bg-cover h-28 rounded-2xl  my-8 flex gap-8 justify-between">
      <div className="h-full text-3xl font-bold items-center inline-flex ml-6">
        Welcome to RAGFlow
      </div>
      <div className="flex justify-between items-center gap-4 mr-5">
        <BannerCard></BannerCard>
        <BannerCard></BannerCard>
        <BannerCard></BannerCard>
        <button
          type="button"
          className="relative p-1 hover:bg-white/10 rounded-full transition-colors"
        >
          <X className="w-6 h-6 text-white" />
        </button>
      </div>
    </section>
  );
}

export function NextBanner() {
  const { t } = useTranslation();
  return (
    <section className="text-5xl pt-10 pb-14 font-bold">
      <span className="text-text-primary">{t('header.welcome')}</span>
      <span className="pl-3 text-transparent bg-clip-text  bg-gradient-to-l from-[#40EBE3] to-[#4A51FF]">
        RAGFlow
      </span>
    </section>
  );
}
