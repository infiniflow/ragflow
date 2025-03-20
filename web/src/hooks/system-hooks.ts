import { useQuery } from '@tanstack/react-query';
import userService from '@/services/user-service';

/**
 * Hook to fetch system configuration including register enable status
 * @returns System configuration with loading status
 */
export const useSystemConfig = () => {
  const { data, isLoading } = useQuery({
    queryKey: ['systemConfig'],
    queryFn: async () => {
      const { data = {} } = await userService.getSystemConfig();
      return data.data || { registerEnable: true }; // Default to enabling registration
    },
  });

  return { config: data, loading: isLoading };
}; 