import { IDataPipelineSelectNode } from '@/components/data-pipeline-select';
import { IconFont } from '@/components/icon-font';
import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { Link } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import LinkDataPipelineModal from './link-data-pipline-modal';
export interface IDataPipelineNodeProps extends IDataPipelineSelectNode {
  isDefault?: boolean;
  linked?: boolean;
}

export interface ILinkDataPipelineProps {
  data?: IDataPipelineNodeProps;
  handleLinkOrEditSubmit?: (data: IDataPipelineNodeProps | undefined) => void;
}

interface DataPipelineItemProps extends IDataPipelineNodeProps {
  openLinkModalFunc?: (open: boolean, data?: IDataPipelineNodeProps) => void;
}

const DataPipelineItem = (props: DataPipelineItemProps) => {
  const { t } = useTranslation();
  const { name, avatar, isDefault, linked, openLinkModalFunc } = props;
  const openUnlinkModal = () => {
    Modal.show({
      visible: true,
      className: '!w-[560px]',
      title: t('dataflowParser.unlinkPipelineModalTitle'),
      children: (
        <div
          className="text-sm text-text-secondary"
          dangerouslySetInnerHTML={{
            __html: t('dataflowParser.unlinkPipelineModalContent'),
          }}
        ></div>
      ),
      onVisibleChange: () => {
        Modal.hide();
      },
      footer: (
        <div className="flex justify-end gap-2">
          <Button variant={'outline'} onClick={() => Modal.hide()}>
            {t('dataflowParser.changeStepModalCancelText')}
          </Button>
          <Button
            variant={'secondary'}
            className="!bg-state-error text-bg-base"
            onClick={() => {
              Modal.hide();
            }}
          >
            {t('dataflowParser.unlinkPipelineModalConfirmText')}
          </Button>
        </div>
      ),
    });
  };

  return (
    <div className="flex items-center justify-between gap-1 px-2 rounded-md border">
      <div className="flex items-center gap-1">
        <RAGFlowAvatar avatar={avatar} name={name} className="size-4" />
        <div>{name}</div>
        {/* {isDefault && (
          <div className="text-xs bg-text-secondary text-bg-base px-2 py-1 rounded-md">
            {t('knowledgeConfiguration.default')}
          </div>
        )} */}
      </div>
      {/* <div className="flex gap-1 items-center">
        <Button
          variant={'transparent'}
          className="border-none"
          type="button"
          onClick={() =>
            openLinkModalFunc?.(true, { ...omit(props, ['openLinkModalFunc']) })
          }
        >
          <Settings2 />
        </Button>
        {!isDefault && (
          <>
            {linked && (
              <Button
                type="button"
                variant={'transparent'}
                className="border-none"
                onClick={() => {
                  openUnlinkModal();
                }}
              >
                <Unlink />
              </Button>
            )}
          </>
        )}
      </div> */}
    </div>
  );
};

const LinkDataPipeline = (props: ILinkDataPipelineProps) => {
  const { data, handleLinkOrEditSubmit: submit } = props;
  const { t } = useTranslation();
  const [openLinkModal, setOpenLinkModal] = useState(false);
  const [currentDataPipeline, setCurrentDataPipeline] =
    useState<IDataPipelineNodeProps>();
  const pipelineNode: IDataPipelineNodeProps[] = useMemo(
    () => [
      {
        id: data?.id,
        name: data?.name,
        avatar: data?.avatar,
        isDefault: data?.isDefault,
        linked: true,
      },
    ],
    [data],
  );
  const openLinkModalFunc = (open: boolean, data?: IDataPipelineNodeProps) => {
    console.log('open', open, data);
    setOpenLinkModal(open);
    if (data) {
      setCurrentDataPipeline(data);
    } else {
      setCurrentDataPipeline(undefined);
    }
  };
  const handleLinkOrEditSubmit = (
    data: IDataPipelineSelectNode | undefined,
  ) => {
    console.log('handleLinkOrEditSubmit', data);
    submit?.(data);
    setOpenLinkModal(false);
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
            {t('knowledgeConfiguration.linkPipelineSetTip')}
          </div>
          <Button
            type="button"
            variant={'transparent'}
            onClick={() => {
              openLinkModalFunc?.(true);
            }}
          >
            <Link />
            <span className="text-xs text-text-primary">
              {t('knowledgeConfiguration.linkDataPipeline')}
            </span>
          </Button>
        </div>
      </section>
      <section className="flex flex-col gap-2">
        {pipelineNode.map(
          (item) =>
            item.id && (
              <DataPipelineItem
                key={item.id}
                openLinkModalFunc={openLinkModalFunc}
                id={item.id}
                name={item.name}
                avatar={item.avatar}
                isDefault={item.isDefault}
                linked={item.linked}
              />
            ),
        )}
      </section>
      <LinkDataPipelineModal
        data={currentDataPipeline}
        open={openLinkModal}
        setOpen={(open: boolean) => {
          openLinkModalFunc(open);
        }}
        onSubmit={handleLinkOrEditSubmit}
      />
    </div>
  );
};
export default LinkDataPipeline;
