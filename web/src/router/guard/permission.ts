export default function setupPermissionGuard(router: Router) {
  router.beforeEach(async (to, from, next) => {
    // TODO: permission logic
    next();
    NProgress.done();
  });
}
