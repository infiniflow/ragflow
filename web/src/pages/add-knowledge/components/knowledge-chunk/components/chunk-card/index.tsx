import { IChunk } from '@/interfaces/database/knowledge';
import { api_host } from '@/utils/api';
import { Card, Flex, Popover, Switch } from 'antd';
import { useDispatch } from 'umi';

import { useState } from 'react';
import styles from './index.less';

interface IProps {
  item: IChunk;
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

const ChunkCard = ({ item }: IProps) => {
  const dispatch = useDispatch();

  const available = Number(item.available_int);
  const [enabled, setEnabled] = useState(available === 1);

  const onChange = (checked: boolean) => {
    setEnabled(checked);
    switchChunk();
  };

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

  return (
    <div>
      <Card>
        <Flex gap={'middle'} justify={'space-between'}>
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

          <section>{item.content_with_weight}</section>
          <div>
            <Switch checked={enabled} onChange={onChange} />
          </div>
        </Flex>
      </Card>
    </div>
  );
};

export default ChunkCard;
