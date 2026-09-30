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
  services: GoServiceStatus[],
): AdminService.ListServicesItem[] =>
  services.map((service) => ({
    id: service.name,
    name: service.name,
    service_type: service.type,
    status: service.status,
    host: service.host || '-',
    port: service.port || '-',
    extra: { elapsed: service.elapsed, message: service.message },
  }));

export const adaptServiceDetail = (
  detail: GoServiceStatus,
): AdminService.ServiceDetail => ({
  service_name: detail.name,
  status: detail.status,
  message: detail,
});
