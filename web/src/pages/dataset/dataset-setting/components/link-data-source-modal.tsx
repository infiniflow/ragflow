import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { IConnector } from '@/interfaces/database/knowledge';
import { useListDataSource } from '@/pages/user-setting/data-source/hooks';
import { IDataSourceBase } from '@/pages/user-setting/data-source/interface';
import { t } from 'i18next';
import { useEffect, useState } from 'react';
import { AddedSourceCard } from './added-source-card';

const LinkDataSourceModal = ({
  selectedList,
  open,
  setOpen,
  onSubmit,
}: {
  selectedList: IConnector[];
  open: boolean;
  setOpen: (open: boolean) => void;
  onSubmit?: (list: IDataSourceBase[] | undefined) => void;
}) => {
  const [list, setList] = useState<IDataSourceBase[]>();
  const [fileterString, setFileterString] = useState('');

  useEffect(() => {
    setList(selectedList);
  }, [selectedList]);

  const { categorizedList } = useListDataSource();
  const handleFormSubmit = (values: any) => {
    console.log(values, selectedList);
    onSubmit?.(list);
  };
  return (
    <Modal
      className="!w-[560px]"
      title={t('knowledgeConfiguration.linkDataSource')}
      open={open}
      onCancel={() => {
        setList(selectedList);
      }}
      onOpenChange={setOpen}
      showfooter={false}
    >
      <div className="flex flex-col gap-4 ">
        {/* {JSON.stringify(selectedList)} */}
        <SearchInput
          value={fileterString}
          onChange={(e) => setFileterString(e.target.value)}
        />
        <div className="flex flex-col gap-3">
          {categorizedList.map((item, index) => (
            <AddedSourceCard
              key={index}
              selectedList={list as IDataSourceBase[]}
              setSelectedList={(list) => setList(list)}
              filterString={fileterString}
              {...item}
            />
          ))}
        </div>
        <div className="flex justify-end gap-1">
          <Button
            type="button"
            variant={'outline'}
            className="btn-primary"
            onClick={() => {
              setOpen(false);
            }}
          >
            {t('modal.cancelText')}
          </Button>
          <Button
            type="button"
            variant={'default'}
            className="btn-primary"
            onClick={handleFormSubmit}
          >
            {t('modal.okText')}
          </Button>
        </div>
      </div>
    </Modal>
  );
};
export default LinkDataSourceModal;
