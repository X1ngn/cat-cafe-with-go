import React from 'react';

interface AvatarProps {
  color: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
  avatar?: string;
}

const sizeMap = {
  sm: 'w-8 h-8',
  md: 'w-12 h-12',
  lg: 'w-16 h-16',
};

export const Avatar: React.FC<AvatarProps> = ({ color, size = 'md', className = '', avatar }) => {
  return (
    <div
      className={`${sizeMap[size]} rounded-full ${className} overflow-hidden flex items-center justify-center`}
      style={{ backgroundColor: color }}
    >
      {avatar ? (
        <img src={avatar} alt="avatar" className="w-full h-full object-cover" />
      ) : null}
    </div>
  );
};
