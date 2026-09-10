/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { IconMap, LLMFactory } from '@/constants/llm';
import { cn } from '@/lib/utils';
import Icon from '@ant-design/icons';
import { IconComponentProps } from '@ant-design/icons/lib/components/Icon';
import { memo, useMemo } from 'react';
import { IconFontFill } from './icon-font';
import { RAGFlowAvatar } from './ragflow-avatar';
import { useIsDarkTheme } from './theme-provider';

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

export const hasSvgIcon = (name: string) =>
  routeList.some((item) => item.name === name);

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
  LLMFactory.MinerUNet,
  LLMFactory.JiekouAI,
  LLMFactory.Perplexity,
];

const svgIcons = [
  LLMFactory.LocalAI,
  // LLMFactory.VolcEngine,
  // LLMFactory.MiniMax,
  LLMFactory.Gemini,
  LLMFactory.StepFun,
  LLMFactory.MinerU,
  LLMFactory.MinerUNet,
  LLMFactory.PaddleOCR,
  LLMFactory.PaddleOCRLocal,
  LLMFactory.N1n,
  // LLMFactory.DeerAPI,
  LLMFactory.Avian,
  LLMFactory.RAGcon,
  LLMFactory.SoMark,
  LLMFactory.NewAPI,
  LLMFactory.Astraflow,
  LLMFactory.AstraflowCN,
  LLMFactory.FuturMix,
  LLMFactory.Xiaomi,
  LLMFactory.YouDao,
  LLMFactory.BAAI,
  LLMFactory.NomicAI,
  LLMFactory.SentenceTransformers,
  LLMFactory.Grok,
  LLMFactory.FastEmbed,
  LLMFactory.HuaweiCloud,
  LLMFactory.OrcaRouter,
  LLMFactory.Qiniu,
  LLMFactory.TokenHub,
  LLMFactory.FunASR,
  LLMFactory.AIMLAPI,
  LLMFactory.GreenPT,
  LLMFactory.Synthorai,
  LLMFactory.MWS,
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
  height?: string | number;
  width?: string | number;
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
