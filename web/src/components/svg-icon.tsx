import Icon from '@ant-design/icons';
import { IconComponentProps } from '@ant-design/icons/lib/components/Icon';

const importAll = (requireContext: __WebpackModuleApi.RequireContext) => {
  const list = requireContext.keys().map((key) => {
    const name = key.replace(/\.\/(.*)\.\w+$/, '$1');
    return { name, value: requireContext(key) };
  });
  return list;
};

let routeList: { name: string; value: string }[] = [];

try {
  routeList = importAll(require.context('@/assets/svg', true, /\.svg$/));
} catch (error) {
  console.warn(error);
  routeList = [];
}

interface IProps extends IconComponentProps {
  name: string;
  width: string | number;
}

const SvgIcon = ({ name, width, ...restProps }: IProps) => {
  const ListItem = routeList.find((item) => item.name === name);
  return (
    <Icon
      component={() => <img src={ListItem?.value} alt="" width={width} />}
      {...(restProps as any)}
    />
  );
};

export default SvgIcon;
