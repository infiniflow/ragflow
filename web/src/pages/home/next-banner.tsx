import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { ArrowRight, X } from 'lucide-react';

const guideCards = [
  {
    badge: 'System',
    title: 'Setting up your LLM',
  },
  {
    badge: 'Chat app',
    title: 'Configuration guides',
  },
  {
    badge: 'Search app',
    title: 'Prompt setting guides',
  },
];

export default function WelcomeGuide(): JSX.Element {
  return (
    <div className="flex w-full max-w-[1800px] items-center gap-4 px-[60px] py-6 relative bg-[#223d8e0d] rounded-3xl overflow-hidden">
      <div
        className="absolute inset-0 bg-gradient-to-r from-pink-300 via-purple-400 to-blue-500 opacity-75"
        style={{
          backgroundSize: 'cover',
          backgroundPosition: 'center',
        }}
      />

      <h1 className="relative flex-1 text-4xl font-bold text-white">
        Welcome to RAGFlow
      </h1>

      <div className="inline-flex items-center gap-[22px] relative">
        {guideCards.map((card, index) => (
          <Card
            key={index}
            className="w-[265px]  backdrop-blur-md border-colors-outline-neutral-standard"
          >
            <CardContent className="flex items-end justify-between p-[15px]">
              <div className="flex flex-col items-start gap-[9px] flex-1">
                <Badge
                  variant="secondary"
                  className="bg-colors-background-core-weak text-colors-text-neutral-strong"
                >
                  {card.badge}
                </Badge>
                <p className="text-lg text-colors-text-neutral-strong">
                  {card.title}
                </p>
              </div>
              <ArrowRight className="w-6 h-6" />
            </CardContent>
          </Card>
        ))}
      </div>

      <button className="relative p-1 hover:bg-white/10 rounded-full transition-colors">
        <X className="w-6 h-6 text-white" />
      </button>
    </div>
  );
}
