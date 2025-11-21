import { useEffect } from 'react';

const BASE_TITLE = 'Claude API 代理管理平台';

/**
 * Hook to set page title
 * @param title - Page specific title
 */
export const usePageTitle = (title: string) => {
  useEffect(() => {
    const previousTitle = document.title;
    document.title = title ? `${title} - ${BASE_TITLE}` : BASE_TITLE;
    
    return () => {
      document.title = previousTitle;
    };
  }, [title]);
};

/**
 * Set page title directly
 * @param title - Page specific title
 */
export const setPageTitle = (title: string) => {
  document.title = title ? `${title} - ${BASE_TITLE}` : BASE_TITLE;
};
