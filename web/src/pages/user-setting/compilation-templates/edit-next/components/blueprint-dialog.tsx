import MarkdownEditor from '@/components/markdown-editor';
import { Modal } from '@/components/ui/modal/modal';
import { Textarea } from '@/components/ui/textarea';
import { IWikiPreset } from '@/interfaces/database/compilation-template';
import { useTranslation } from 'react-i18next';

type BlueprintDialogProps = {
  preset?: IWikiPreset;
  onOpenChange: (open: boolean) => void;
};

export function BlueprintDialog({ preset, onOpenChange }: BlueprintDialogProps) {
  const { t } = useTranslation();

  return (
    <Modal
      open={Boolean(preset)}
      onOpenChange={onOpenChange}
      title={preset?.topic}
      size="large"
      showfooter={false}
      destroyOnClose
    >
      <div className="flex flex-col gap-4">
        <div className="space-y-2">
          <div className="text-sm text-text-secondary">
            {t('setting.instruction')}
          </div>
          <Textarea value={preset?.instruction ?? ''} rows={6} readOnly />
        </div>

        <div className="flex h-[50vh] min-h-0 flex-col">
          <MarkdownEditor content={preset?.page_example ?? ''} readOnly />
        </div>
      </div>
    </Modal>
  );
}
