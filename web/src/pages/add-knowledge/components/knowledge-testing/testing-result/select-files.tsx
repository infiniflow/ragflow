import { ReactComponent as NavigationPointerIcon } from '@/assets/svg/navigation-pointer.svg';
import { ITestingDocument } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { Table, TableProps } from 'antd';
import { useDispatch, useSelector } from 'umi';

interface IProps {
  handleTesting: () => Promise<any>;
}

const SelectFiles = ({ handleTesting }: IProps) => {
  const documents: ITestingDocument[] = useSelector(
    (state: any) => state.testingModel.documents,
  );

  const dispatch = useDispatch();

  const columns: TableProps<ITestingDocument>['columns'] = [
    {
      title: 'Name',
      dataIndex: 'doc_name',
      key: 'doc_name',
      render: (text) => <p>{text}</p>,
    },

    {
      title: 'Hits',
      dataIndex: 'count',
      key: 'count',
      width: 80,
    },
    {
      title: 'View',
      key: 'view',
      width: 50,
      render: (_, { doc_id }) => (
        <a
          href={`${api_host}/document/get/${doc_id}`}
          target="_blank"
          rel="noreferrer"
        >
          <NavigationPointerIcon />
        </a>
      ),
    },
  ];

  const rowSelection = {
    onChange: (selectedRowKeys: React.Key[]) => {
      dispatch({
        type: 'testingModel/setSelectedDocumentIds',
        payload: selectedRowKeys,
      });
      handleTesting();
    },
    getCheckboxProps: (record: ITestingDocument) => ({
      disabled: record.doc_name === 'Disabled User', // Column configuration not to be checked
      name: record.doc_name,
    }),
  };

  return (
    <Table
      columns={columns}
      dataSource={documents}
      showHeader={false}
      rowSelection={rowSelection}
      rowKey={'doc_id'}
    />
  );
};

export default SelectFiles;
