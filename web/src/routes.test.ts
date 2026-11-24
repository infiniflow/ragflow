import { Routes, routers } from './routes';

describe('user setting routes', () => {
  it('renders settings outside the global root layout', () => {
    const settingsRoute = routers.routes.find(
      (route) => route.path === Routes.UserSetting,
    );
    const globalRootRoute = routers.routes.find(
      (route) =>
        route.path === Routes.Root &&
        route.children?.some((child) => child.path === Routes.Agents),
    );

    expect(settingsRoute).toBeDefined();
    expect(globalRootRoute).toBeDefined();
    expect(
      globalRootRoute?.children?.some(
        (route) => route.path === Routes.UserSetting,
      ) ?? false,
    ).toBe(false);
  });
});
