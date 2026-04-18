import { useFetchTokenListBeforeOtherStep } from '@/components/embed-dialog/use-show-embed-dialog';
import { Button } from '@/components/ui/button';
import { SharedFrom } from '@/constants/chat';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { Send } from 'lucide-react';
import { useState } from 'react';
import { useFetchSearchDetail } from '../next-searches/hooks';
import EmbedAppModal from './embed-app-modal';

function EmbedIcon() {
  const [openEmbed, setOpenEmbed] = useState(false);
  const { beta, handleOperate } = useFetchTokenListBeforeOtherStep();

  const { data: SearchData } = useFetchSearchDetail();

  return (
    <>
      <Button
        variant={'outline'}
        onClick={() => {
          handleOperate().then((res) => {
            if (res) {
              setOpenEmbed(!openEmbed);
            }
          });
        }}
      >
        <Send />
      </Button>
      <EmbedAppModal
        open={openEmbed}
        setOpen={setOpenEmbed}
        url={Routes.SearchShare}
        token={SearchData?.id as string}
        from={SharedFrom.Search}
        beta={beta}
      />
    </>
  );
}

export function RAGFlowLogo({
  onClick,
  showEmbedIcon = true,
}: {
  onClick?: React.MouseEventHandler<HTMLHeadingElement>;
  showEmbedIcon?: boolean;
}) {
  return (
    <div className="flex gap-4 items-center">
      <h1
        onClick={onClick}
        className={cn(
          'text-4xl font-bold bg-gradient-to-l from-[#40EBE3] to-[#4A51FF] bg-clip-text',
        )}
      >
        RAGFlow
      </h1>
      {showEmbedIcon && <EmbedIcon></EmbedIcon>}
    </div>
  );
}
