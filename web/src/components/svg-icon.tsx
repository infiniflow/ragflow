import { IconMap, LLMFactory } from '@/constants/llm';
import { cn } from '@/lib/utils';
import Icon, { UserOutlined } from '@ant-design/icons';
import { IconComponentProps } from '@ant-design/icons/lib/components/Icon';
import { Avatar } from 'antd';
import { AvatarSize } from 'antd/es/avatar/AvatarContext';
import { useMemo } from 'react';
import { IconFontFill } from './icon-font';
import { useIsDarkTheme } from './theme-provider';

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
  height?: string | number;
  imgClass?: string;
}

const SvgIcon = ({ name, width, height, imgClass, ...restProps }: IProps) => {
  const ListItem = routeList.find((item) => item.name === name);
  return (
    <Icon
      component={() => (
        <img
          src={ListItem?.value}
          alt=""
          width={width}
          height={height}
          className={cn(imgClass, 'max-w-full')}
        />
      )}
      {...(restProps as any)}
    />
  );
};

export const LlmIcon = ({
  name,
  height = 48,
  width = 48,
  size = 'large',
  imgClass,
}: {
  name: string;
  height?: number;
  width?: number;
  size?: AvatarSize;
  imgClass?: string;
}) => {
  const isDark = useIsDarkTheme();
  const themeIcons = [
    LLMFactory.FishAudio,
    LLMFactory.TogetherAI,
    LLMFactory.Meituan,
    LLMFactory.Longcat,
  ];
  let icon = useMemo(() => {
    const icontemp = IconMap[name as keyof typeof IconMap];
    if (themeIcons.includes(name as LLMFactory)) {
      if (isDark) {
        return icontemp + '-dark';
      } else {
        return icontemp + '-bright';
      }
    }
    return icontemp;
  }, [name, isDark]);

  const svgIcons = [
    LLMFactory.LocalAI,
    // LLMFactory.VolcEngine,
    // LLMFactory.MiniMax,
    LLMFactory.Gemini,
    LLMFactory.StepFun,
    // LLMFactory.DeerAPI,
  ];
  if (svgIcons.includes(name as LLMFactory)) {
    return (
      <SvgIcon
        name={`llm/${icon}`}
        width={width}
        height={height}
        imgClass={imgClass}
      ></SvgIcon>
    );
  }

  return icon ? (
    <IconFontFill
      name={icon}
      className={cn('size-8 flex items-center justify-center', imgClass)}
    />
  ) : (
    <IconFontFill
      name={'moxing-default'}
      className={cn('size-8 flex items-center justify-center', imgClass)}
    />
    // <Avatar shape="square" size={size} icon={<UserOutlined />} />
  );
};

export const HomeIcon = ({
  name,
  height = '32',
  width = '32',
  size = 'large',
  imgClass,
}: {
  name: string;
  height?: string;
  width?: string;
  size?: AvatarSize;
  imgClass?: string;
}) => {
  const isDark = useIsDarkTheme();
  const icon = isDark ? name : `${name}-bri`;

  return icon ? (
    <SvgIcon
      name={`home-icon/${icon}`}
      width={width}
      height={height}
      imgClass={imgClass}
    ></SvgIcon>
  ) : (
    <Avatar shape="square" size={size} icon={<UserOutlined />} />
  );
};

export default SvgIcon;
