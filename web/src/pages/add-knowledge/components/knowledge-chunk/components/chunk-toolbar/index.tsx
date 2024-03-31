import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { KnowledgeRouteKey } from '@/constants/knowledge';
import { useKnowledgeBaseId } from '@/hooks/knowledgeHook';
import {
  ArrowLeftOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  DeleteOutlined,
  DownOutlined,
  FilePdfOutlined,
  PlusOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import {
  Button,
  Checkbox,
  Flex,
  Input,
  Menu,
  MenuProps,
  Popover,
  Radio,
  RadioChangeEvent,
  Space,
} from 'antd';
import { ChangeEventHandler, useCallback, useMemo, useState } from 'react';
import { Link, useDispatch, useSelector } from 'umi';
import { ChunkModelState } from '../../model';

interface IProps {
  checked: boolean;
  getChunkList: () => void;
  selectAllChunk: (checked: boolean) => void;
  createChunk: () => void;
  removeChunk: () => void;
  switchChunk: (available: number) => void;
}

const ChunkToolBar = ({
  getChunkList,
  selectAllChunk,
  checked,
  createChunk,
  removeChunk,
  switchChunk,
}: IProps) => {
  const { documentInfo, available, searchString }: ChunkModelState =
    useSelector((state: any) => state.chunkModel);
  const dispatch = useDispatch();
  const knowledgeBaseId = useKnowledgeBaseId();
  const [isShowSearchBox, setIsShowSearchBox] = useState(false);

  const handleSelectAllCheck = useCallback(
    (e: any) => {
      selectAllChunk(e.target.checked);
    },
    [selectAllChunk],
  );

  const handleSearchIconClick = () => {
    setIsShowSearchBox(true);
  };

  const handleSearchChange: ChangeEventHandler<HTMLInputElement> = (e) => {
    const val = e.target.value;
    dispatch({ type: 'chunkModel/setSearchString', payload: val });
    dispatch({
      type: 'chunkModel/throttledGetChunkList',
      payload: documentInfo.id,
    });
  };

  const handleSearchBlur = () => {
    if (!searchString.trim()) {
      setIsShowSearchBox(false);
    }
  };

  const handleDelete = useCallback(() => {
    removeChunk();
  }, [removeChunk]);

  const handleEnabledClick = useCallback(() => {
    switchChunk(1);
  }, [switchChunk]);

  const handleDisabledClick = useCallback(() => {
    switchChunk(0);
  }, [switchChunk]);

  const items: MenuProps['items'] = useMemo(() => {
    return [
      {
        key: '1',
        label: (
          <>
            <Checkbox onChange={handleSelectAllCheck} checked={checked}>
              <b>Select All</b>
            </Checkbox>
          </>
        ),
      },
      { type: 'divider' },
      {
        key: '2',
        label: (
          <Space onClick={handleEnabledClick}>
            <CheckCircleOutlined />
            <b>Enabled Selected</b>
          </Space>
        ),
      },
      {
        key: '3',
        label: (
          <Space onClick={handleDisabledClick}>
            <CloseCircleOutlined />
            <b>Disabled Selected</b>
          </Space>
        ),
      },
      { type: 'divider' },
      {
        key: '4',
        label: (
          <Space onClick={handleDelete}>
            <DeleteOutlined />
            <b>Delete Selected</b>
          </Space>
        ),
      },
    ];
  }, [
    checked,
    handleSelectAllCheck,
    handleDelete,
    handleEnabledClick,
    handleDisabledClick,
  ]);

  const content = (
    <Menu style={{ width: 200 }} items={items} selectable={false} />
  );

  const handleFilterChange = (e: RadioChangeEvent) => {
    selectAllChunk(false);
    dispatch({ type: 'chunkModel/setAvailable', payload: e.target.value });
    getChunkList();
  };

  const filterContent = (
    <Radio.Group onChange={handleFilterChange} value={available}>
      <Space direction="vertical">
        <Radio value={undefined}>All</Radio>
        <Radio value={1}>Enabled</Radio>
        <Radio value={0}>Disabled</Radio>
      </Space>
    </Radio.Group>
  );

  return (
    <Flex justify="space-between" align="center">
      <Space size={'middle'}>
        <Link
          to={`/knowledge/${KnowledgeRouteKey.Dataset}?id=${knowledgeBaseId}`}
        >
          <ArrowLeftOutlined />
        </Link>
        <FilePdfOutlined />
        {documentInfo.name}
      </Space>
      <Space>
        <Popover content={content} placement="bottom" arrow={false}>
          <Button>
            Bulk
            <DownOutlined />
          </Button>
        </Popover>
        {isShowSearchBox ? (
          <Input
            size="middle"
            placeholder="Search"
            prefix={<SearchOutlined />}
            allowClear
            onChange={handleSearchChange}
            onBlur={handleSearchBlur}
            value={searchString}
          />
        ) : (
          <Button icon={<SearchOutlined />} onClick={handleSearchIconClick} />
        )}

        <Popover content={filterContent} placement="bottom" arrow={false}>
          <Button icon={<FilterIcon />} />
        </Popover>
        <Button
          icon={<PlusOutlined />}
          type="primary"
          onClick={() => createChunk()}
        />
      </Space>
    </Flex>
  );
};

export default ChunkToolBar;
