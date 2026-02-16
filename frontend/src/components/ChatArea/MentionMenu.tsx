import React, { useEffect, useState } from 'react';
import { useAppStore } from '@/stores/appStore';
import { Cat } from '@/types';
import { Avatar } from '@/components/common/Avatar';
import { StatusBadge } from '@/components/common/StatusBadge';
import { catAPI } from '@/services/api';

interface MentionMenuProps {
  onSelect: (catId: string, catName: string) => void;
}

export const MentionMenu: React.FC<MentionMenuProps> = ({ onSelect }) => {
  const { cats, setCats, mentionQuery } = useAppStore();
  const [filteredCats, setFilteredCats] = useState<Cat[]>([]);

  useEffect(() => {
    loadCats();
  }, []);

  useEffect(() => {
    if (mentionQuery) {
      setFilteredCats(
        cats.filter((cat) => cat.name.toLowerCase().includes(mentionQuery.toLowerCase()))
      );
    } else {
      setFilteredCats(cats);
    }
  }, [mentionQuery, cats]);

  const loadCats = async () => {
    try {
      const response = await catAPI.getCats();
      setCats(response.data);
      setFilteredCats(response.data);
    } catch (error) {
      console.error('Failed to load cats:', error);
    }
  };

  return (
    <div className="absolute bottom-20 left-8 w-[300px] bg-white border border-gray-200 rounded-xl shadow-lg overflow-hidden">
      {filteredCats.map((cat) => (
        <div
          key={cat.id}
          onClick={() => onSelect(cat.id, cat.name)}
          className="flex items-center justify-between px-4 py-3 hover:bg-gray-50 cursor-pointer"
        >
          <div className="flex items-center gap-3">
            <Avatar color={cat.color} size="sm" className="rounded-2xl" />
            <span className="font-medium">{cat.name}</span>
          </div>
          <StatusBadge status={cat.status} />
        </div>
      ))}
    </div>
  );
};
