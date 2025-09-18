import { IconFont } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Link, Settings2, Unlink } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import LinkDataPipelineModal from './link-data-pipline-modal';

interface DataPipelineItemProps {
  name: string;
  avatar?: string;
  isDefault?: boolean;
  linked?: boolean;
  openLinkModalFunc?: (open: boolean) => void;
}
const DataPipelineItem = (props: DataPipelineItemProps) => {
  const { t } = useTranslation();
  const { name, avatar, isDefault, linked, openLinkModalFunc } = props;
  return (
    <div className="flex items-center justify-between gap-1 px-2 rounded-lg border">
      <div className="flex items-center gap-1">
        <RAGFlowAvatar avatar={avatar} name={name} className="size-4" />
        <div>{name}</div>
        {isDefault && (
          <div className="text-xs bg-text-secondary text-bg-base px-2 py-1 rounded-md">
            {t('knowledgeConfiguration.default')}
          </div>
        )}
      </div>
      <div className="flex gap-1 items-center">
        <Button variant={'transparent'} className="border-none">
          <Settings2 />
        </Button>
        {!isDefault && (
          <Button
            variant={'transparent'}
            className="border-none"
            onClick={() => {
              openLinkModalFunc?.(true);
            }}
          >
            {linked ? <Link /> : <Unlink />}
          </Button>
        )}
      </div>
    </div>
  );
};
const LinkDataPipeline = () => {
  const { t } = useTranslation();
  const [openLinkModal, setOpenLinkModal] = useState(false);
  const testNode = [
    {
      name: 'Data Pipeline 1',
      avatar: 'https://avatars.githubusercontent.com/u/10656201?v=4',
      isDefault: true,
      linked: true,
    },
    {
      name: 'Data Pipeline 2',
      avatar: 'https://avatars.githubusercontent.com/u/10656201?v=4',
      linked: false,
    },
  ];
  const openLinkModalFunc = (open: boolean) => {
    setOpenLinkModal(open);
  };
  return (
    <div className="flex flex-col gap-2">
      <section className="flex flex-col">
        <div className="flex items-center gap-1 text-text-primary text-sm">
          <IconFont name="Pipeline" />
          {t('knowledgeConfiguration.dataPipeline')}
        </div>
        <div className="flex justify-between items-center">
          <div className="text-center text-xs text-text-secondary">
            Manage data pipeline linkage with this dataset
          </div>
          <Button variant={'transparent'}>
            <Link />
            <span className="text-xs text-text-primary">
              {t('knowledgeConfiguration.linkDataPipeline')}
            </span>
          </Button>
        </div>
      </section>
      <section className="flex flex-col gap-2">
        {testNode.map((item) => (
          <DataPipelineItem
            key={item.name}
            openLinkModalFunc={openLinkModalFunc}
            {...item}
          />
        ))}
      </section>
      <LinkDataPipelineModal
        open={openLinkModal}
        setOpen={(open: boolean) => {
          openLinkModalFunc(open);
        }}
      />
    </div>
  );
};
export default LinkDataPipeline;
