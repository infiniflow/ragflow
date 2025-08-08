import React from 'react';

interface SpotlightProps {
  className?: string;
}

const Spotlight: React.FC<SpotlightProps> = ({ className }) => {
  return (
    <div
      className={`absolute inset-0  opacity-80 ${className} rounded-lg`}
      style={{
        backdropFilter: 'blur(30px)',
        zIndex: -1,
      }}
    >
      <div
        className="absolute inset-0"
        style={{
          background:
            'radial-gradient(circle at 50% 190%, #fff4 0%, #fff0 60%)',
          pointerEvents: 'none',
        }}
      ></div>
    </div>
  );
};

export default Spotlight;
