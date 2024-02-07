import { IChunk } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { Card, Checkbox, CheckboxProps, Flex, Popover, Switch } from 'antd';
import { useDispatch } from 'umi';

import { useState } from 'react';
import styles from './index.less';

interface IProps {
  item: IChunk;
  checked: boolean;
  editChunk: (chunkId: string) => void;
  handleCheckboxClick: (chunkId: string, checked: boolean) => void;
}

interface IImage {
  id: string;
  className: string;
}
// Pass onMouseEnter and onMouseLeave to img tag using props
const Image = ({ id, className, ...props }: IImage) => {
  return (
    <img
      {...props}
      src={`${api_host}/document/image/${id}`}
      alt=""
      className={className}
    />
  );
};

const ChunkCard = ({
  item,
  checked,
  handleCheckboxClick,
  editChunk,
}: IProps) => {
  const dispatch = useDispatch();

  const available = Number(item.available_int);
  const [enabled, setEnabled] = useState(available === 1);

  const switchChunk = () => {
    dispatch({
      type: 'chunkModel/switch_chunk',
      payload: {
        chunk_ids: [item.chunk_id],
        available_int: available === 0 ? 1 : 0,
        doc_id: item.doc_id,
      },
    });
  };

  const onChange = (checked: boolean) => {
    setEnabled(checked);
    switchChunk();
  };

  const handleCheck: CheckboxProps['onChange'] = (e) => {
    handleCheckboxClick(item.chunk_id, e.target.checked);
  };

  const handleContentClick = () => {
    editChunk(item.chunk_id);
  };

  return (
    <div>
      <Card>
        <Flex gap={'middle'} justify={'space-between'}>
          <Checkbox onChange={handleCheck} checked={checked}></Checkbox>
          {item.img_id && (
            <Popover
              placement="topRight"
              content={
                <Image id={item.img_id} className={styles.imagePreview}></Image>
              }
            >
              <img
                src={`${api_host}/document/image/${item.img_id}`}
                alt=""
                className={styles.image}
              />
              <Image id={item.img_id} className={styles.image}></Image>
            </Popover>
          )}

          <section
            onDoubleClick={handleContentClick}
            className={styles.content}
            dangerouslySetInnerHTML={{ __html: item.content_with_weight }}
          >
            {/* {item.content_with_weight} */}
          </section>
          <div>
            <Switch checked={enabled} onChange={onChange} />
          </div>
        </Flex>
      </Card>
    </div>
  );
};

export default ChunkCard;
