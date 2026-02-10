import { useQuery } from '@tanstack/react-query';
import axios from 'axios';
import { useState } from 'react';

export interface PublicDialog {
  id: string;
  name: string;
  description: string;
  icon: string;
  language: string;
  prologue: string;
  creator_name: string;
  creator_avatar: string;
  shared_id: string;
  auth_token: string;
  create_time: number;
  update_time: number;
}

export interface PublicDialogListResponse {
  dialogs: PublicDialog[];
  total: number;
  page: number;
  page_size: number;
}

export const useFetchPublicDialogs = (
  page: number = 1,
  pageSize: number = 20,
  keywords: string = '',
) => {
  return useQuery<PublicDialogListResponse>({
    queryKey: ['publicDialogs', page, pageSize, keywords],
    queryFn: async () => {
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: pageSize.toString(),
      });

      if (keywords) {
        params.append('keywords', keywords);
      }

      // Changed to POST to match /v1/dialog/next endpoint
      const response = await axios.post(
        `/v1/dialog/public/list?${params.toString()}`,
        {}, // Empty body
      );

      return response.data.data;
    },
  });
};

export const useSearchKeywords = () => {
  const [keywords, setKeywords] = useState('');
  const [debouncedKeywords, setDebouncedKeywords] = useState('');

  const handleSearch = (value: string) => {
    setKeywords(value);
    // Debounce search
    const timer = setTimeout(() => {
      setDebouncedKeywords(value);
    }, 500);
    return () => clearTimeout(timer);
  };

  return {
    keywords,
    debouncedKeywords,
    handleSearch,
  };
};

export const generateShareUrl = (dialog: PublicDialog, theme?: string) => {
  const { protocol, host } = window.location;
  const params = new URLSearchParams({
    shared_id: dialog.id, // Use dialog.id as shared_id
    from: 'chat',
    auth: dialog.auth_token,
    embedded: 'true', // Add embedded parameter to hide header
  });

  if (theme) {
    params.append('theme', theme);
  }

  return `${protocol}//${host}/next-chats/share?${params.toString()}`;
};
