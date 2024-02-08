import { api_host } from '@/utils/api';

interface IImage {
  id: string;
  className: string;
}

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

export default Image;
