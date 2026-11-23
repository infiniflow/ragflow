import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
import { Button } from '@/components/ui/button';
import { SharedFrom } from '@/constants/chat';
import { Routes } from '@/routes';
import { LucideSend } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useFetchSearchDetail } from '../next-searches/hooks';
import EmbedAppModal from './embed-app-modal';

export function SearchEmbedButton() {
  const { t } = useTranslation();
  const { data: searchData } = useFetchSearchDetail();
  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  return (
    <>
      <Button
        variant="ghost"
        onClick={showEmbedModal}
        data-testid="search-detail-embed-open"
      >
        <LucideSend />
        {t('common.embedIntoSite')}
      </Button>
      <EmbedAppModal
        open={embedVisible}
        setOpen={hideEmbedModal}
        url={Routes.SearchShare}
        token={searchData?.id as string}
        from={SharedFrom.Search}
        beta={beta}
      />
    </>
  );
}
