export const personalRoutes = ['/home', '/computers', '/settings'] as const;

export function isPersonalArea(pathname: string): boolean {
  return personalRoutes.some((route) => pathname === route || pathname.startsWith(`${route}/`));
}

export function isPersonalRouteActive(pathname: string, route: string): boolean {
  return pathname === route || pathname.startsWith(`${route}/`);
}
