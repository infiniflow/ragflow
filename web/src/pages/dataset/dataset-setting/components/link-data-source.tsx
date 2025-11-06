import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { IConnector } from '@/interfaces/database/knowledge';
import { DataSourceInfo } from '@/pages/user-setting/data-source/contant';
import { IDataSourceBase } from '@/pages/user-setting/data-source/interface';
import { Link, Settings, Unlink } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import LinkDataSourceModal from './link-data-source-modal';

export type IDataSourceNodeProps = IConnector & {
  icon: React.ReactNode;
};

export interface ILinkDataSourceProps {
  data?: IConnector[];
  handleLinkOrEditSubmit?: (data: IDataSourceBase[] | undefined) => void;
  unbindFunc?: (item: DataSourceItemProps) => void;
}

interface DataSourceItemProps extends IDataSourceNodeProps {
  openLinkModalFunc?: (open: boolean, data?: IDataSourceNodeProps) => void;
  unbindFunc?: (item: DataSourceItemProps) => void;
}

const DataSourceItem = (props: DataSourceItemProps) => {
  const { t } = useTranslation();
  const { id, name, icon, openLinkModalFunc, unbindFunc } = props;

  const { navigateToDataSourceDetail } = useNavigatePage();
  const toDetail = (id: string) => {
    navigateToDataSourceDetail(id);
  };
  const openUnlinkModal = () => {
    Modal.show({
      visible: true,
      className: '!w-[560px]',
      title: t('dataflowParser.unlinkSourceModalTitle'),
      children: (
        <div
          className="text-sm text-text-secondary"
          dangerouslySetInnerHTML={{
            __html: t('dataflowParser.unlinkSourceModalContent'),
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
              unbindFunc?.(props);
              Modal.hide();
            }}
          >
            {t('dataflowParser.unlinkSourceModalConfirmText')}
          </Button>
        </div>
      ),
    });
  };

  return (
    <div className="flex items-center justify-between gap-1 px-2 rounded-md border ">
      <div className="flex items-center gap-1">
        {icon}
        <div>{name}</div>
      </div>
      <div className="flex gap-1 items-center">
        <Button
          variant={'transparent'}
          className="border-none"
          type="button"
          onClick={() => {
            toDetail(id);
          }}
          // onClick={() =>
          //   openLinkModalFunc?.(true, { ...omit(props, ['openLinkModalFunc']) })
          // }
        >
          <Settings />
        </Button>
        <>
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
        </>
      </div>
    </div>
  );
};

const LinkDataSource = (props: ILinkDataSourceProps) => {
  const { data, handleLinkOrEditSubmit: submit, unbindFunc } = props;
  const { t } = useTranslation();
  const [openLinkModal, setOpenLinkModal] = useState(false);

  const pipelineNode: IDataSourceNodeProps[] = useMemo(() => {
    if (data && data.length > 0) {
      return data.map((item) => {
        return {
          ...item,
          id: item?.id,
          name: item?.name,
          icon:
            DataSourceInfo[item?.source as keyof typeof DataSourceInfo]?.icon ||
            '',
        } as IDataSourceNodeProps;
      });
    }
    return [];
  }, [data]);

  const openLinkModalFunc = (open: boolean, data?: IDataSourceNodeProps) => {
    console.log('open', open, data);
    setOpenLinkModal(open);
    // if (data) {
    //   setCurrentDataSource(data);
    // } else {
    //   setCurrentDataSource(undefined);
    // }
  };

  const handleLinkOrEditSubmit = (data: IDataSourceBase[] | undefined) => {
    console.log('handleLinkOrEditSubmit', data);
    submit?.(data);
    setOpenLinkModal(false);
  };

  return (
    <div className="flex flex-col gap-2">
      <section className="flex flex-col">
        <div className="flex items-center gap-1 text-text-primary text-sm">
          {t('knowledgeConfiguration.dataSource')}
        </div>
        <div className="flex justify-between items-center">
          <div className="text-center text-xs text-text-secondary">
            {t('knowledgeConfiguration.linkSourceSetTip')}
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
              {t('knowledgeConfiguration.linkDataSource')}
            </span>
          </Button>
        </div>
      </section>
      <section className="flex flex-col gap-2">
        {pipelineNode.map(
          (item) =>
            item.id && (
              <DataSourceItem
                key={item.id}
                openLinkModalFunc={openLinkModalFunc}
                unbindFunc={unbindFunc}
                {...item}
              />
            ),
        )}
      </section>
      <LinkDataSourceModal
        selectedList={data as IConnector[]}
        open={openLinkModal}
        setOpen={(open: boolean) => {
          openLinkModalFunc(open);
        }}
        onSubmit={handleLinkOrEditSubmit}
      />
    </div>
  );
};
export default LinkDataSource;
