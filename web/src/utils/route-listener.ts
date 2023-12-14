const emitter = mitt();

const key = Symbol('ROUTE_CHANGE');

let latestRoute: RouteLocationNormalized;

export function setRouteEmitter(to: RouteLocationNormalized) {
  emitter.emit(key, to);
  latestRoute = to;
}

export function listenerRouteChange(
  handler: (route: RouteLocationNormalized) => void,
  immediate = true,
) {
  emitter.on(key, handler as Handler);
  if (immediate && latestRoute) {
    handler(latestRoute);
  }
}

export function removeRouteListener() {
  emitter.off(key);
}
