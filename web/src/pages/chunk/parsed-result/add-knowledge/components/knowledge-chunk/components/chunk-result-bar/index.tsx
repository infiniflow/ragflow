import { ReactComponent as FilterIcon } from '@/assets/filter.svg';
import { useTranslate } from '@/hooks/common-hooks';
import { PlusOutlined, SearchOutlined } from '@ant-design/icons';
import {
  Button,
  Input,
  Popover,
  Radio,
  RadioChangeEvent,
  Segmented,
  SegmentedProps,
  Space,
} from 'antd';
import { ChunkTextMode } from '../../constant';

export default ({
  changeChunkTextMode,
  available,
  selectAllChunk,
  handleSetAvailable,
  createChunk,
  handleInputChange,
  searchString,
}) => {
  const { t } = useTranslate('chunk');

  const handleFilterChange = (e: RadioChangeEvent) => {
    selectAllChunk(false);
    handleSetAvailable(e.target.value);
  };
  const filterContent = (
    <Radio.Group onChange={handleFilterChange} value={available}>
      <Space direction="vertical">
        <Radio value={undefined}>{t('all')}</Radio>
        <Radio value={1}>{t('enabled')}</Radio>
        <Radio value={0}>{t('disabled')}</Radio>
      </Space>
    </Radio.Group>
  );

  return (
    <div className="flex pr-[25px]">
      <Segmented
        options={[
          { label: t(ChunkTextMode.Full), value: ChunkTextMode.Full },
          { label: t(ChunkTextMode.Ellipse), value: ChunkTextMode.Ellipse },
        ]}
        onChange={changeChunkTextMode as SegmentedProps['onChange']}
      />
      <div className="ml-auto"></div>
      <Input
        style={{ width: 200 }}
        size="middle"
        placeholder={t('search')}
        prefix={<SearchOutlined />}
        allowClear
        onChange={handleInputChange}
        value={searchString}
      />
      <div className="w-[20px]"></div>
      <Popover content={filterContent} placement="bottom" arrow={false}>
        <Button icon={<FilterIcon />} />
      </Popover>
      <div className="w-[20px]"></div>
      <Button
        icon={<PlusOutlined />}
        type="primary"
        onClick={() => createChunk()}
      />
    </div>
  );
};
