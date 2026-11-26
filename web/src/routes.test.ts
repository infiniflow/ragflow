import { Routes, routers } from './routes';

describe('standalone page routes', () => {
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

  it('renders agent templates outside the global root layout', () => {
    const templatesRoute = routers.routes.find(
      (route) => route.path === Routes.AgentTemplates,
    );
    const globalRootRoute = routers.routes.find(
      (route) =>
        route.path === Routes.Root &&
        route.children?.some((child) => child.path === Routes.Agents),
    );

    expect(templatesRoute).toBeDefined();
    expect(globalRootRoute).toBeDefined();
    expect(
      globalRootRoute?.children?.some(
        (route) => route.path === Routes.AgentTemplates,
      ) ?? false,
    ).toBe(false);
  });
});

describe('agent editor routes', () => {
  it('registers Version history under the agent layout', () => {
    const editorRootRoute = routers.routes.find(
      (route) =>
        route.path === Routes.Root &&
        route.children?.some((child) => child.path === Routes.Agent),
    );
    const agentRoute = editorRootRoute?.children?.find(
      (route) => route.path === Routes.Agent,
    );
    const agentLayoutRoute = agentRoute?.children?.find(
      (route) => route.path === `${Routes.Agent}/:id`,
    );

    expect(
      agentLayoutRoute?.children?.some(
        (route) => route.path === Routes.AgentVersionHistory,
      ),
    ).toBe(true);
  });
});
