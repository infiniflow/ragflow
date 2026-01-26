import { IconMap, LLMFactory } from '@/constants/llm';
import { cn } from '@/lib/utils';
import Icon from '@ant-design/icons';
import { IconComponentProps } from '@ant-design/icons/lib/components/Icon';
import { memo, useMemo } from 'react';
import { IconFontFill } from './icon-font';
import { RAGFlowAvatar } from './ragflow-avatar';
import { useIsDarkTheme } from './theme-provider';

// const importAll = (requireContext: __WebpackModuleApi.RequireContext) => {
//   const list = requireContext.keys().map((key) => {
//     const name = key.replace(/\.\/(.*)\.\w+$/, '$1');
//     return { name, value: requireContext(key) };
//   });
//   return list;
// };

// let routeList: { name: string; value: string }[] = [];

// try {
//   routeList = importAll(require.context('@/assets/svg', true, /\.svg$/));
// } catch (error) {
//   console.warn(error);
//   routeList = [];
// }

const svgModules = import.meta.glob('@/assets/svg/**/*.svg', {
  eager: true,
  query: '?url',
});

const routeList: { name: string; value: string }[] = Object.entries(
  svgModules,
).map(([path, module]) => {
  const name = path.replace(/^.*\/assets\/svg\//, '').replace(/\.[^/.]+$/, '');
  // @ts-ignore
  return { name, value: module.default || module };
});

interface IProps extends IconComponentProps {
  name: string;
  width: string | number;
  height?: string | number;
  imgClass?: string;
}

const SvgIcon = memo(
  ({ name, width, height, imgClass, ...restProps }: IProps) => {
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
  },
);

SvgIcon.displayName = 'SvgIcon';

const themeIcons = [
  LLMFactory.FishAudio,
  LLMFactory.TogetherAI,
  LLMFactory.Meituan,
  LLMFactory.Longcat,
  LLMFactory.MinerU,
];

const svgIcons = [
  LLMFactory.LocalAI,
  // LLMFactory.VolcEngine,
  // LLMFactory.MiniMax,
  LLMFactory.Gemini,
  LLMFactory.StepFun,
  LLMFactory.MinerU,
  LLMFactory.PaddleOCR,
  LLMFactory.N1n,
  // LLMFactory.DeerAPI,
];

export const LlmIcon = ({
  name,
  height = 48,
  width = 48,
  imgClass,
}: {
  name: string;
  height?: number;
  width?: number;
  imgClass?: string;
}) => {
  const isDark = useIsDarkTheme();
  const icon = useMemo(() => {
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
  );
};

export const HomeIcon = ({
  name,
  height = '32',
  width = '32',
  imgClass,
}: {
  name: string;
  height?: string;
  width?: string;
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
    <RAGFlowAvatar avatar={'user'}></RAGFlowAvatar>
  );
};

export default SvgIcon;
