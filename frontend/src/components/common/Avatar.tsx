import React from 'react';

interface AvatarProps {
  color: string;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizeMap = {
  sm: 'w-8 h-8',
  md: 'w-12 h-12',
  lg: 'w-16 h-16',
};

export const Avatar: React.FC<AvatarProps> = ({ color, size = 'md', className = '' }) => {
  return (
    <div
      className={`${sizeMap[size]} rounded-full ${className}`}
      style={{ backgroundColor: color }}
    />
  );
};
