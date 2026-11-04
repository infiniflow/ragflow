import { pickByBackend } from '@/utils/backend-variant';

export type GoServiceStatus = {
  type: string;
  name: string;
  host: string;
  port: number;
  status: string;
  elapsed: string;
  message: string;
};

export const adaptServiceList = (
  services: AdminService.ListServicesItem[] | GoServiceStatus[],
): AdminService.ListServicesItem[] =>
  pickByBackend({
    go: () =>
      (services as GoServiceStatus[]).map((service) => ({
        id: service.name,
        name: service.name,
        service_type: service.type,
        status: service.status,
        host: service.host || '-',
        port: service.port || '-',
        extra: { elapsed: service.elapsed, message: service.message },
      })),
    python: () => services as AdminService.ListServicesItem[],
  })();

export const adaptServiceDetail = (
  detail: AdminService.ServiceDetail | GoServiceStatus,
): AdminService.ServiceDetail =>
  pickByBackend({
    go: () => {
      const service = detail as GoServiceStatus;
      return {
        service_name: service.name,
        status: service.status,
        message: service,
      };
    },
    python: () => detail as AdminService.ServiceDetail,
  })();
