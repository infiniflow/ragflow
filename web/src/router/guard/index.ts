import setupUserLoginInfoGuard from './userLoginInfo';
import setupPermissionGuard from './permission';

function setupPageGuard(router: Router) {
  router.beforeEach(async (to) => {
    // emit route change
    setRouteEmitter(to);
  });
}

export default function createRouteGuard(router: Router) {
  setupPageGuard(router);
  setupUserLoginInfoGuard(router);
  setupPermissionGuard(router);
}
