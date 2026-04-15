import { useFetchTokenListBeforeOtherStep } from '@/components/embed-dialog/use-show-embed-dialog';
import { Button } from '@/components/ui/button';
import { SharedFrom } from '@/constants/chat';
import { useFetchTenantInfo } from '@/hooks/use-user-setting-request';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import { Send } from 'lucide-react';
import { useState } from 'react';
import { useFetchSearchDetail } from '../next-searches/hooks';
import EmbedAppModal from './embed-app-modal';

export function RAGFlowLogo({
  onClick,
}: {
  onClick?: React.MouseEventHandler<HTMLHeadingElement>;
}) {
  const [openEmbed, setOpenEmbed] = useState(false);
  const { beta, handleOperate } = useFetchTokenListBeforeOtherStep();
  const { data: tenantInfo } = useFetchTenantInfo();
  const tenantId = tenantInfo.tenant_id;
  const { data: SearchData } = useFetchSearchDetail();

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
        tenantId={tenantId}
        beta={beta}
      />
    </div>
  );
}
