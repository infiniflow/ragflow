import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { MultiSelect } from '@/components/ui/multi-select';
import { Textarea } from '@/components/ui/textarea';
import { useCreateEvaluationDataset } from '@/hooks/use-evaluation-request';
import { useFetchKnowledgeList } from '@/hooks/use-knowledge-request';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

type Props = {
  visible: boolean;
  hideModal: () => void;
  onSuccess?: (datasetId: string) => void;
};

export function CreateEvaluationDatasetDialog({
  visible,
  hideModal,
  onSuccess,
}: Props) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [kbIds, setKbIds] = useState<string[]>([]);
  const { list: kbs } = useFetchKnowledgeList();
  const { mutateAsync, isPending } = useCreateEvaluationDataset();

  useEffect(() => {
    if (!visible) {
      setName('');
      setDescription('');
      setKbIds([]);
    }
  }, [visible]);

  const kbOptions =
    kbs?.map((kb) => ({ label: kb.name, value: kb.id })) ?? [];

  const handleOk = async () => {
    if (!name.trim() || kbIds.length === 0) {
      return;
    }
    const res = await mutateAsync({
      name: name.trim(),
      description: description.trim(),
      kb_ids: kbIds,
    });
    hideModal();
    onSuccess?.(res.id);
  };

  return (
    <Dialog open={visible} onOpenChange={(open) => !open && hideModal()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('evaluation.createDataset')}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label>{t('evaluation.name')}</Label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('evaluation.datasetNamePlaceholder')}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>{t('evaluation.description')}</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={3}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>{t('evaluation.knowledgeBases')}</Label>
            <MultiSelect
              options={kbOptions}
              value={kbIds}
              onValueChange={setKbIds}
              placeholder={t('evaluation.selectKnowledgeBases')}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={hideModal}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleOk}
            disabled={!name.trim() || kbIds.length === 0 || isPending}
          >
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
