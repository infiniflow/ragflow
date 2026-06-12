import './index.less';
const aspectRatio = {
  top: 240,
  middle: 466,
  bottom: 704,
};

export const BgSvg = ({ isPaused = false }: { isPaused?: boolean }) => {
  const animationClass = isPaused ? 'paused' : '';

  const def = (
    path: string,
    id: number | string = '',
    type: keyof typeof aspectRatio,
  ) => {
    return (
      <svg
        className="w-full h-full"
        // style={{ aspectRatio: `1440/${aspectRatio[type]}` }}
        // preserveAspectRatio="xMinYMid meet"
        preserveAspectRatio="none"
        // viewBox={`${getPathBounds(path).minX} 0 ${
        //   getPathBounds(path).width
        // } ${height}`}
        viewBox={`0 0 1440 ${aspectRatio[type]}`}
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <linearGradient id={`glow${id}`} x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor="#80FFF8" stopOpacity="0" />
            <stop offset="50%" stopColor="#80FFF8" stopOpacity="1" />
            <stop offset="100%" stopColor="#80FFF8" stopOpacity="0" />
          </linearGradient>
          <linearGradient
            id="strokeWidthGradient"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="0%"
          >
            <stop offset="0%" stopColor="#000" />
            <stop offset="10%" stopColor="#fff" />
            <stop offset="50%" stopColor="#fff" />
            <stop offset="90%" stopColor="#fff" />
            <stop offset="100%" stopColor="#000" />
          </linearGradient>

          <linearGradient
            id={`highlight${id}`}
            x1="0%"
            y1="0%"
            x2="100%"
            y2="0%"
          >
            <stop offset="45%" stopColor="#FFF" stopOpacity="0.2" />
            <stop offset="48%" stopColor="#FFD700" stopOpacity="0.3" />
          </linearGradient>

          <filter
            id={`glowFilter${id}`}
            x="-10%"
            y="-10%"
            width="120%"
            height="120%"
          >
            <feGaussianBlur in="SourceGraphic" stdDeviation="5.2" />
            {/* <feBlend
              in="blur"
              in2="SourceGraphic"
              mode="screen"
              result="glow"
            /> */}
          </filter>
          <filter
            id={`highlightFilter${id}`}
            x="-5%"
            y="-5%"
            width="110%"
            height="110%"
          >
            <feGaussianBlur in="SourceGraphic" stdDeviation="5.5" />
          </filter>
          <mask id={`glowMask${id}`}>
            <rect width="100%" height="100%" fill="transparent" />
            <path
              d={path}
              fill="none"
              stroke="url(#strokeWidthGradient)"
              strokeWidth="1"
              strokeDasharray="50,600"
              strokeDashoffset="0"
              filter={`url(#glowFilter${id})`}
              className="animate-glow mask-path"
            />
            <path
              d={path}
              fill="none"
              stroke={`url(#highlight${id})`}
              strokeWidth="0.5"
              strokeDasharray="50,600"
              strokeDashoffset="16"
              filter={`url(#highlightFilter${id})`}
              className="animate-highlight mask-path"
            />
          </mask>
        </defs>
        <path
          d={path}
          stroke="#00BEB4"
          strokeWidth="1"
          fill="none"
          opacity="0.1"
        />
        <path
          d={path}
          stroke={`url(#glow${id})`}
          strokeWidth="2"
          fill="none"
          opacity="0.8"
          mask={`url(#glowMask${id})`}
        />
      </svg>
    );
  };
  return (
    <div className="absolute inset-0 overflow-hidden pointer-events-none" />
  );
};
